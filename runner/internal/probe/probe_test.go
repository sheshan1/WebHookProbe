package probe

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func scenario(url string) Scenario {
	return Scenario{Version: 1, Name: "test", TargetURL: url, Headers: map[string]string{"Content-Type": "application/json"}, Body: `{"id":"event-1"}`, Copies: 1, Concurrency: 1, TimeoutMs: 1000, ExpectedStatuses: []int{200}}
}
func completed(t *testing.T, s Scenario) Report {
	t.Helper()
	r, e := Start(context.Background(), s)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-r.Done():
	case <-time.After(3 * time.Second):
		r.Cancel()
		t.Fatal("run did not finish")
	}
	return r.Snapshot()
}
func TestValidationAndJSONPointers(t *testing.T) {
	s := scenario("http://127.0.0.1")
	data, _ := json.Marshal(s)
	if _, e := Decode(data); e != nil {
		t.Fatal(e)
	}
	for _, input := range []string{`{}`, string(data) + ` {}`, strings.Replace(string(data), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(data), `"version":1`, `"unknown":1,"version":1`, 1)} {
		if _, e := Decode([]byte(input)); e == nil {
			t.Fatalf("accepted invalid input: %s", input)
		}
	}
	for _, bad := range []Scenario{func() Scenario { x := s; x.Copies = 0; return x }(), func() Scenario { x := s; x.TargetURL = "http://user:secret@localhost"; return x }(), func() Scenario { x := s; x.Headers = map[string]string{"X-Test": "x\r\nInjected: true"}; return x }()} {
		if bad.Validate() == nil {
			t.Fatal("invalid scenario accepted")
		}
	}
	root := map[string]any{"a/b": []any{map[string]any{"~key": json.Number("1.0")}}}
	v, e := pointer(root, "/a~1b/0/~0key")
	if e != nil || !equals(v, json.Number("1")) {
		t.Fatal("pointer/numeric equality broken")
	}
	if _, e := pointer(root, "/missing"); e == nil {
		t.Fatal("missing field matched")
	}
}
func TestConcurrentBoundedDeliveryAndExactPayload(t *testing.T) {
	var active, maxActive, total atomic.Int32
	arrived := make(chan struct{}, 3)
	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		total.Add(1)
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"id":"event-1"}` || r.Header.Get("X-Check") != "present" {
			t.Error("payload/header changed")
		}
		arrived <- struct{}{}
		<-release
		w.Write([]byte("ok"))
	}))
	defer target.Close()
	s := scenario(target.URL)
	s.Copies = 3
	s.Concurrency = 3
	s.Headers["X-Check"] = "present"
	run, e := Start(context.Background(), s)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-arrived:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("concurrency not reached")
		}
	}
	close(release)
	<-run.Done()
	if maxActive.Load() != 3 || total.Load() != 3 || !run.Snapshot().DeliveryPassed {
		t.Fatal("incorrect concurrency or attempt count")
	}
	s.Concurrency = 1
	s.Copies = 1 // exercise sequencing separately without the gated handler
}
func TestSequentialUnexpectedStatusesAndPreviewLimits(t *testing.T) {
	var active, maxActive atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		if n > maxActive.Load() {
			maxActive.Store(n)
		}
		w.WriteHeader(202)
		w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer target.Close()
	s := scenario(target.URL)
	s.Copies = 5
	s.Concurrency = 1
	r := completed(t, s)
	if maxActive.Load() != 1 || len(r.Attempts) != 5 || r.DeliveryPassed || r.Verdict != "failed" || !r.Attempts[0].Truncated || len(r.Attempts[0].Preview) != 4096 {
		t.Fatal("sequential/status/capture boundary failure")
	}
}
func TestTimeoutUnavailableAndCancellation(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer target.Close()
	s := scenario(target.URL)
	s.TimeoutMs = 30
	r := completed(t, s)
	if r.Attempts[0].Outcome != "timeout" || r.Attempts[0].Status != nil {
		t.Fatal("timeout fabricated response")
	}
	s.Copies = 100
	s.Concurrency = 2
	s.TimeoutMs = 1000
	run, _ := Start(context.Background(), s)
	run.Cancel()
	<-run.Done()
	if run.Snapshot().State != "cancelled" || len(run.Snapshot().Attempts) >= 100 {
		t.Fatal("cancellation did not stop scheduling")
	}
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	address := listener.Addr().String()
	listener.Close()
	s = scenario("http://" + address)
	r = completed(t, s)
	if r.Attempts[0].Outcome != "transport_error" || r.Business.Outcome != "not_configured" {
		t.Fatal("unavailable incorrectly verified")
	}
}
func TestVerificationOutcomes(t *testing.T) {
	for _, kind := range []string{"match", "mismatch", "unusable", "missing", "delayed"} {
		t.Run(kind, func(t *testing.T) {
			var start atomic.Int64
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Write([]byte("ok"))
					return
				}
				switch kind {
				case "unusable":
					w.Write([]byte("not json"))
				case "missing":
					w.Write([]byte(`{}`))
				case "mismatch":
					w.Write([]byte(`{"count":0}`))
				case "delayed":
					now := time.Now().UnixNano()
					start.CompareAndSwap(0, now)
					if time.Duration(now-start.Load()) < 40*time.Millisecond {
						w.Write([]byte(`{"count":0}`))
					} else {
						w.Write([]byte(`{"count":1}`))
					}
				default:
					w.Write([]byte(`{"count":1}`))
				}
			}))
			defer target.Close()
			s := scenario(target.URL)
			s.Verification = &Verification{URL: target.URL, Assertions: []Assertion{{Pointer: "/count", Equals: json.Number("1")}}, PollIntervalMs: 10, TimeoutMs: 200, RequestTimeoutMs: 50, StableForMs: 30}
			r := completed(t, s)
			expected := "passed"
			if kind == "mismatch" {
				expected = "failed"
			}
			if kind == "unusable" || kind == "missing" {
				expected = "inconclusive"
			}
			if r.Business.Outcome != expected {
				t.Fatalf("got %s, expected %s", r.Business.Outcome, expected)
			}
		})
	}
}
func TestHeaderSecretsNeverAppearInReports(t *testing.T) {
	secret := "test-super-secret-token"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("Authorization") + " " + secret))
	}))
	defer target.Close()
	s := scenario(target.URL)
	s.Headers["Authorization"] = "Bearer " + secret
	s.Description = secret
	r := completed(t, s)
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), secret) {
		t.Fatal("report leaked header credential")
	}
	if !strings.Contains(r.Attempts[0].Preview, "[REDACTED]") {
		t.Fatal("redaction absent")
	}
}
func TestAPILifecycleAndRequestDisconnect(t *testing.T) {
	gate := make(chan struct{})
	arrived := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { arrived <- struct{}{}; <-gate; w.Write([]byte("ok")) }))
	defer target.Close()
	api := NewServer()
	server := httptest.NewServer(api)
	defer server.Close()
	defer api.Shutdown()
	s := scenario(target.URL)
	data, _ := json.Marshal(s)
	res, e := http.Post(server.URL+"/v1/runs", "application/json", strings.NewReader(string(data)))
	if e != nil {
		t.Fatal(e)
	}
	var report Report
	json.NewDecoder(res.Body).Decode(&report)
	res.Body.Close()
	if res.StatusCode != 202 {
		t.Fatal("start not accepted")
	}
	<-arrived
	res, _ = http.Post(server.URL+"/v1/runs", "application/json", strings.NewReader(string(data)))
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Fatal("admitted overlapping run")
	}
	close(gate)
	api.mu.Lock()
	run := api.runs[report.ID]
	api.mu.Unlock()
	<-run.Done()
	if run.Snapshot().State != "completed" {
		t.Fatal("HTTP disconnect cancelled background run")
	}
	res, _ = http.Get(server.URL + "/v1/runs/" + report.ID + "/report")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("report unavailable")
	}
	fresh := httptest.NewRecorder()
	NewServer().ServeHTTP(fresh, httptest.NewRequest("GET", "/v1/runs/"+report.ID, nil))
	if fresh.Code != 404 {
		t.Fatal("restart retained memory")
	}
}

func TestRedactionPreservesStructureWithShortSecrets(t *testing.T) {
	s := scenario("http://localhost/path?token=hidden")
	s.Headers["Authorization"] = "a"
	s.Name = "a valid name"
	s.Body = `{"value":"a"}`
	redacted := s.Redacted()
	if redacted.Version != 1 || redacted.Copies != 1 || redacted.TargetURL == "" || redacted.Headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("redaction damaged structure: %+v", redacted)
	}
	data, _ := json.Marshal(redacted)
	if !json.Valid(data) || strings.Contains(string(data), "token=hidden") {
		t.Fatal("unsafe redaction")
	}
}
func TestObservationEvidenceAndPreciseNumbers(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Write([]byte(`{"count":9007199254740993}`))
	}))
	defer target.Close()
	s := scenario(target.URL)
	s.Verification = &Verification{URL: target.URL, Headers: map[string]string{}, Assertions: []Assertion{{Pointer: "/count", Equals: json.Number("9007199254740993")}}, PollIntervalMs: 10, TimeoutMs: 100, RequestTimeoutMs: 50, StableForMs: 0}
	r := completed(t, s)
	if r.Verdict != "passed" || len(r.Business.Observations) != 1 || len(r.Business.Observations[0].Checks) != 1 {
		t.Fatalf("missing evidence: %+v", r.Business)
	}
	check := r.Business.Observations[0].Checks[0]
	if check.Actual != json.Number("9007199254740993") || !check.Matched || !check.Found {
		t.Fatalf("lost precision: %+v", check)
	}
	s.Verification.Assertions[0].Equals = json.Number("9007199254740992")
	if completed(t, s).Business.Outcome != "failed" {
		t.Fatal("rounded distinct values into equality")
	}
}

func TestObservationLimitRetainsFinalEvidence(t *testing.T) {
	var polls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.Copy(io.Discard, r.Body)
			w.Write([]byte("ok"))
			return
		}
		count := 0
		if polls.Add(1) > 205 {
			count = 1
		}
		json.NewEncoder(w).Encode(map[string]int{"count": count})
	}))
	defer target.Close()
	s := scenario(target.URL)
	s.Verification = &Verification{URL: target.URL, Headers: map[string]string{}, Assertions: []Assertion{{Pointer: "/count", Equals: json.Number("1")}}, PollIntervalMs: 10, TimeoutMs: 10000, RequestTimeoutMs: 1000, StableForMs: 0}
	run, err := Start(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Done():
	case <-time.After(12 * time.Second):
		run.Cancel()
		t.Fatal("verification did not finish")
	}
	result := run.Snapshot()
	if result.Business.Outcome != "passed" || len(result.Business.Observations) != 200 || result.Business.DroppedObservations == 0 {
		t.Fatalf("retention: %+v", result.Business)
	}
	last := result.Business.Observations[199]
	if last.Outcome != "match" || !last.Checks[0].Matched {
		t.Fatal("final matching observation was discarded")
	}
}
