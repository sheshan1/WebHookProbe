package probe

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Assertion struct {
	Pointer string `json:"pointer"`
	Equals  any    `json:"equals"`
}
type Verification struct {
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Assertions       []Assertion       `json:"assertions"`
	PollIntervalMs   int               `json:"pollIntervalMs"`
	TimeoutMs        int               `json:"timeoutMs"`
	RequestTimeoutMs int               `json:"requestTimeoutMs"`
	StableForMs      int               `json:"stableForMs"`
}
type Scenario struct {
	Version          int               `json:"version"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	TargetURL        string            `json:"targetUrl"`
	Headers          map[string]string `json:"headers"`
	Body             string            `json:"body"`
	Copies           int               `json:"copies"`
	Concurrency      int               `json:"concurrency"`
	TimeoutMs        int               `json:"timeoutMs"`
	ExpectedStatuses []int             `json:"expectedStatuses"`
	Verification     *Verification     `json:"verification,omitempty"`
}

func Decode(data []byte) (Scenario, error) {
	var s Scenario
	if !utf8.Valid(data) {
		return s, errors.New("scenario must be valid UTF-8")
	}
	if len(data) > 262144 {
		return s, errors.New("scenario exceeds 256 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(&s); err != nil {
		return s, errors.New("invalid scenario JSON or unknown field")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return s, errors.New("scenario must contain one JSON object")
	}
	// Reject duplicate keys before an attacker can make validation and execution disagree.
	tokens := json.NewDecoder(bytes.NewReader(data))
	var visit func() error
	visit = func() error {
		t, e := tokens.Token()
		if e != nil {
			return e
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for tokens.More() {
				k, e := tokens.Token()
				if e != nil {
					return e
				}
				key := k.(string)
				if seen[key] {
					return errors.New("duplicate scenario key")
				}
				seen[key] = true
				if e := visit(); e != nil {
					return e
				}
			}
		} else if delim == '[' {
			for tokens.More() {
				if e := visit(); e != nil {
					return e
				}
			}
		}
		_, e = tokens.Token()
		return e
	}
	if err := visit(); err != nil {
		return s, err
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(data, &fields)
	for _, key := range []string{"version", "name", "targetUrl", "headers", "body", "copies", "concurrency", "timeoutMs", "expectedStatuses"} {
		raw, ok := fields[key]
		if !ok || string(raw) == "null" {
			return s, errors.New("missing required scenario field: " + key)
		}
	}
	if raw, ok := fields["verification"]; ok {
		if string(raw) == "null" {
			return s, errors.New("omit verification instead of null")
		}
		var verification struct {
			Assertions []map[string]json.RawMessage `json:"assertions"`
		}
		json.Unmarshal(raw, &verification)
		for _, a := range verification.Assertions {
			if raw, ok := a["pointer"]; !ok || string(raw) == "null" {
				return s, errors.New("every assertion needs a pointer string")
			}
			if _, ok := a["equals"]; !ok {
				return s, errors.New("every assertion needs equals, including explicit null")
			}
		}
	}
	if s.Verification != nil && s.Verification.Headers == nil {
		s.Verification.Headers = map[string]string{}
	}
	return s, s.Validate()
}

func validURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func headersOK(headers map[string]string) bool {
	seen := map[string]bool{}
	for k, v := range headers {
		canonical := http.CanonicalHeaderKey(k)
		if seen[canonical] || len(k) > 128 || len(v) > 4096 {
			return false
		}
		seen[canonical] = true
		if k == "" || strings.ContainsAny(v, "\r\n") || v == "[REDACTED]" {
			return false
		}
		for _, c := range k {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
				return false
			}
		}
		switch strings.ToLower(k) {
		case "host", "content-length", "transfer-encoding", "connection", "upgrade", "proxy-connection":
			return false
		}
	}
	return len(headers) <= 32
}
func (s Scenario) Validate() error {
	if s.Version != 1 || strings.TrimSpace(s.Name) == "" || len(s.Name) > 120 || len(s.Description) > 2000 {
		return errors.New("version must be 1; name required (max 120), description max 2000")
	}
	if !validURL(s.TargetURL) || !headersOK(s.Headers) {
		return errors.New("use an HTTP(S) target without URL credentials/fragment and valid request headers; re-enter redacted secrets")
	}
	if len(s.Body) > 65536 || s.Copies < 1 || s.Copies > 1000 || s.Concurrency < 1 || s.Concurrency > min(s.Copies, 50) || s.TimeoutMs < 10 || s.TimeoutMs > 30000 {
		return errors.New("body max 64 KiB; copies 1–1000; concurrency 1–min(copies,50); timeout 10–30000 ms")
	}
	if len(s.ExpectedStatuses) == 0 || len(s.ExpectedStatuses) > 100 {
		return errors.New("expectedStatuses is required")
	}
	for _, v := range s.ExpectedStatuses {
		if v < 100 || v > 599 {
			return errors.New("invalid expected status")
		}
	}
	if v := s.Verification; v != nil {
		if !validURL(v.URL) || !headersOK(v.Headers) || len(v.Assertions) < 1 || len(v.Assertions) > 20 {
			return errors.New("verification requires an HTTP(S) URL, valid headers, and 1–20 assertions")
		}
		if v.TimeoutMs < 100 || v.TimeoutMs > 30000 || v.RequestTimeoutMs < 10 || v.RequestTimeoutMs > v.TimeoutMs || v.PollIntervalMs < 10 || v.PollIntervalMs > 5000 || v.StableForMs < 0 || v.StableForMs >= v.TimeoutMs {
			return errors.New("invalid verification deadlines, polling interval, or stability window")
		}
		for _, a := range v.Assertions {
			if len(a.Pointer) > 512 {
				return errors.New("assertion pointer exceeds 512 bytes")
			}
			switch a.Equals.(type) {
			case nil, string, bool, json.Number, float64:
			default:
				return errors.New("assertions compare scalar JSON values only")
			}
			if e := validatePointer(a.Pointer); e != nil {
				return e
			}
		}
	}
	return nil
}
func pointer(root any, path string) (any, error) {
	if path == "" {
		return root, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("invalid pointer")
	}
	for _, part := range strings.Split(path[1:], "/") {
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 == len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return nil, errors.New("invalid pointer")
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch obj := root.(type) {
		case map[string]any:
			var ok bool
			root, ok = obj[part]
			if !ok {
				return nil, errors.New("missing field")
			}
		case []any:
			i, e := strconv.Atoi(part)
			if e != nil || i < 0 || i >= len(obj) || strconv.Itoa(i) != part {
				return nil, errors.New("missing index")
			}
			root = obj[i]
		default:
			return nil, errors.New("missing field")
		}
	}
	return root, nil
}
func equals(a, b any) bool {
	if x, ok := a.(json.Number); ok {
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		xr, xok := new(big.Rat).SetString(string(x))
		yr, yok := new(big.Rat).SetString(string(y))
		return xok && yok && xr.Cmp(yr) == 0
	}
	return reflect.DeepEqual(a, b)
}
func safeURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return "invalid URL"
	}
	u.User = nil
	if u.RawQuery != "" {
		u.RawQuery = "redacted"
	}
	return u.String()
}
func sensitive(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Content-Type", "Accept", "X-Demo-Generation", "X-Request-Id":
		return false
	}
	return true
}
func (s Scenario) scrub(value string) string {
	maps := []map[string]string{s.Headers}
	if s.Verification != nil {
		maps = append(maps, s.Verification.Headers)
	}
	for _, headers := range maps {
		for k, v := range headers {
			if sensitive(k) && v != "" {
				value = strings.ReplaceAll(value, v, "[REDACTED]")
				q, _ := json.Marshal(v)
				if len(q) > 2 {
					value = strings.ReplaceAll(value, string(q[1:len(q)-1]), "[REDACTED]")
				}
				if strings.HasPrefix(v, "Bearer ") {
					value = strings.ReplaceAll(value, strings.TrimPrefix(v, "Bearer "), "[REDACTED]")
				}
			}
		}
	}
	return value
}
func (s Scenario) Redacted() Scenario {
	data, _ := json.Marshal(s)
	var copy Scenario
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.Decode(&copy)
	copy.Name = s.scrub(copy.Name)
	copy.Description = s.scrub(copy.Description)
	copy.TargetURL = s.scrub(safeURL(copy.TargetURL))
	copy.Body = s.scrub(copy.Body)
	for k, v := range copy.Headers {
		if sensitive(k) {
			copy.Headers[k] = "[REDACTED]"
		} else {
			copy.Headers[k] = s.scrub(v)
		}
	}
	if copy.Verification != nil {
		copy.Verification.URL = s.scrub(safeURL(copy.Verification.URL))
		for k, v := range copy.Verification.Headers {
			if sensitive(k) {
				copy.Verification.Headers[k] = "[REDACTED]"
			} else {
				copy.Verification.Headers[k] = s.scrub(v)
			}
		}
		for i, a := range copy.Verification.Assertions {
			if text, ok := a.Equals.(string); ok {
				copy.Verification.Assertions[i].Equals = s.scrub(text)
			}
		}
	}
	return copy
}
func validatePointer(path string) error {
	if path != "" && !strings.HasPrefix(path, "/") {
		return errors.New("invalid pointer")
	}
	for i := 0; i < len(path); i++ {
		if path[i] == '~' {
			if i+1 == len(path) || (path[i+1] != '0' && path[i+1] != '1') {
				return errors.New("invalid pointer")
			}
			i++
		}
	}
	return nil
}
