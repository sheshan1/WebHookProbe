package probe

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Attempt struct {
	ID         int       `json:"id"`
	StartedAt  time.Time `json:"startedAt"`
	DurationMs int64     `json:"durationMs"`
	Status     *int      `json:"status"`
	Outcome    string    `json:"outcome"`
	Preview    string    `json:"preview"`
	Truncated  bool      `json:"truncated"`
}
type Check struct {
	Pointer  string `json:"pointer"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Found    bool   `json:"found"`
	Matched  bool   `json:"matched"`
}
type Observation struct {
	Checks  []Check   `json:"checks,omitempty"`
	At      time.Time `json:"at"`
	Outcome string    `json:"outcome"`
}
type Business struct {
	Outcome             string        `json:"outcome"`
	Observations        []Observation `json:"observations"`
	DroppedObservations int           `json:"droppedObservations"`
	Detail              string        `json:"detail"`
}
type Report struct {
	Version        int        `json:"version"`
	ID             string     `json:"id"`
	State          string     `json:"state"`
	Verdict        string     `json:"verdict"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	Scenario       Scenario   `json:"scenario"`
	Attempts       []Attempt  `json:"attempts"`
	Business       Business   `json:"business"`
	Scheduled      int        `json:"scheduled"`
	Unstarted      int        `json:"unstarted"`
	DeliveryPassed bool       `json:"deliveryPassed"`
}
type Run struct {
	mu     sync.Mutex
	report Report
	cancel context.CancelFunc
	done   chan struct{}
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (r *Run) Snapshot() Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, _ := json.Marshal(r.report)
	var out Report
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.Decode(&out)
	return out
}
func (r *Run) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.report.FinishedAt == nil {
		r.cancel()
	}
}
func (r *Run) Done() <-chan struct{}   { return r.done }
func (r *Run) mutate(fn func(*Report)) { r.mu.Lock(); defer r.mu.Unlock(); fn(&r.report) }
func Start(parent context.Context, s Scenario) (*Run, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// Freeze mutable maps/slices, including assertion values, before starting workers.
	data, _ := json.Marshal(s)
	frozen, err := Decode(data)
	if err != nil {
		return nil, err
	}
	s = frozen
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	r := &Run{report: Report{Version: 1, ID: NewID(), State: "running", Verdict: "pending", StartedAt: time.Now().UTC(), Scenario: s.Redacted(), Attempts: []Attempt{}, Business: Business{Outcome: "not_configured", Observations: []Observation{}}}, cancel: cancel, done: make(chan struct{})}
	go r.execute(ctx, s)
	return r, nil
}
func client() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxIdleConns: 60, MaxIdleConnsPerHost: 50, ResponseHeaderTimeout: 30 * time.Second, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (r *Run) execute(ctx context.Context, s Scenario) {
	defer close(r.done)
	defer r.cancel()
	c := client()
	defer c.CloseIdleConnections()
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < s.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if ctx.Err() != nil {
					continue
				}
				r.mutate(func(p *Report) { p.Scheduled++ })
				a := send(ctx, c, s, id)
				r.mutate(func(p *Report) { p.Attempts = append(p.Attempts, a) })
			}
		}()
	}
schedule:
	for i := 1; i <= s.Copies; i++ {
		select {
		case <-ctx.Done():
			break schedule
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	r.mutate(func(p *Report) {
		sort.Slice(p.Attempts, func(i, j int) bool { return p.Attempts[i].ID < p.Attempts[j].ID })
		p.Unstarted = s.Copies - p.Scheduled
		p.DeliveryPassed = len(p.Attempts) == s.Copies
		for _, a := range p.Attempts {
			p.DeliveryPassed = p.DeliveryPassed && a.Outcome == "expected_status"
		}
	})
	if s.Verification != nil && ctx.Err() == nil {
		r.mutate(func(p *Report) { p.State = "verifying" })
		r.verify(ctx, c, s)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.report.FinishedAt = &now
	if ctx.Err() != nil {
		r.report.State = "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			r.report.State = "failed"
		}
		r.report.Verdict = "inconclusive"
		if s.Verification != nil && r.report.Business.Outcome != "passed" {
			r.report.Business.Outcome = "inconclusive"
			r.report.Business.Detail = "Run interrupted; receiver effects may still commit."
		}
	} else {
		r.report.State = "completed"
		switch {
		case !r.report.DeliveryPassed || r.report.Business.Outcome == "failed":
			r.report.Verdict = "failed"
		case r.report.Business.Outcome == "passed":
			r.report.Verdict = "passed"
		default:
			r.report.Verdict = "inconclusive"
		}
	}
}
func send(ctx context.Context, c *http.Client, s Scenario, id int) Attempt {
	start := time.Now()
	a := Attempt{ID: id, StartedAt: start.UTC()}
	deadline, cancel := context.WithTimeout(ctx, time.Duration(s.TimeoutMs)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(deadline, http.MethodPost, s.TargetURL, strings.NewReader(s.Body))
	if err != nil {
		a.Outcome = "request_error"
		return a
	}
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		a.Outcome = "transport_error"
		if deadline.Err() != nil {
			a.Outcome = "timeout"
		}
		if ctx.Err() != nil {
			a.Outcome = "cancelled"
		}
	} else {
		status := res.StatusCode
		a.Status = &status
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 4097))
		res.Body.Close()
		a.Truncated = len(body) > 4096
		body = body[:min(len(body), 4096)]
		a.Preview = s.scrub(strings.ToValidUTF8(string(body), "�"))
		a.Outcome = "unexpected_status"
		for _, expected := range s.ExpectedStatuses {
			if status == expected {
				a.Outcome = "expected_status"
			}
		}
		if readErr != nil {
			a.Outcome = "response_read_error"
			if deadline.Err() != nil {
				a.Outcome = "timeout"
			}
		}
	}
	a.DurationMs = time.Since(start).Milliseconds()
	return a
}
func (r *Run) verify(parent context.Context, c *http.Client, s Scenario) {
	v := s.Verification
	ctx, cancel := context.WithTimeout(parent, time.Duration(v.TimeoutMs)*time.Millisecond)
	defer cancel()
	var stable time.Time
	hadMismatch := false
	hadErrors := false
	for ctx.Err() == nil {
		requestCtx, stop := context.WithTimeout(ctx, time.Duration(v.RequestTimeoutMs)*time.Millisecond)
		req, _ := http.NewRequestWithContext(requestCtx, http.MethodGet, v.URL, nil)
		for k, val := range v.Headers {
			req.Header.Set(k, val)
		}
		outcome := "unavailable"
		checks := []Check{}
		res, err := c.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(res.Body, 65537))
			res.Body.Close()
			if readErr == nil && len(body) <= 65536 && res.StatusCode >= 200 && res.StatusCode < 300 {
				d := json.NewDecoder(bytes.NewReader(body))
				d.UseNumber()
				var root any
				if d.Decode(&root) == nil && d.Decode(new(any)) == io.EOF {
					outcome = "match"
					for _, assertion := range v.Assertions {
						value, e := pointer(root, assertion.Pointer)
						matched := e == nil && equals(value, assertion.Equals)
						expected := assertion.Equals
						switch value.(type) {
						case map[string]any, []any:
							value = "[non-scalar JSON value]"
						}
						if text, ok := value.(string); ok {
							value = s.scrub(text)
							if len(value.(string)) > 512 {
								value = value.(string)[:512] + "…"
							}
						}
						if text, ok := expected.(string); ok {
							expected = s.scrub(text)
						}
						checks = append(checks, Check{Pointer: s.scrub(assertion.Pointer), Expected: expected, Actual: value, Found: e == nil, Matched: matched})
						if e != nil {
							outcome = "unusable"
							break
						}
						if !matched {
							outcome = "mismatch"
						}
					}
				} else {
					outcome = "unusable"
				}
			}
		}
		stop()
		if ctx.Err() != nil {
			break
		}
		at := time.Now()
		r.mutate(func(p *Report) {
			if len(p.Business.Observations) == 200 {
				copy(p.Business.Observations, p.Business.Observations[1:])
				p.Business.Observations = p.Business.Observations[:199]
				p.Business.DroppedObservations++
			}
			p.Business.Observations = append(p.Business.Observations, Observation{At: at.UTC(), Outcome: outcome, Checks: checks})
		})
		if outcome == "match" {
			if stable.IsZero() {
				stable = at
			}
			if time.Since(stable) >= time.Duration(v.StableForMs)*time.Millisecond && ctx.Err() == nil {
				r.mutate(func(p *Report) {
					p.Business.Outcome = "passed"
					p.Business.Detail = "Configured assertions matched over the observation window."
				})
				return
			}
		} else {
			stable = time.Time{}
			if outcome == "mismatch" {
				hadMismatch = true
			} else {
				hadErrors = true
			}
		}
		timer := time.NewTimer(time.Duration(v.PollIntervalMs) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	r.mutate(func(p *Report) {
		p.Business.Outcome = "inconclusive"
		p.Business.Detail = "No stable conclusion before the verification deadline."
		if hadMismatch && !hadErrors {
			p.Business.Outcome = "failed"
			p.Business.Detail = "Readable state did not satisfy the required stable assertions."
		}
	})
}
