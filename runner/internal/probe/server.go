package probe

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Server struct {
	mu     sync.Mutex
	runs   map[string]*Run
	order  []string
	active *Run
	closed bool
}

func NewServer() *Server { return &Server{runs: map[string]*Run{}} }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, map[string]any{"status": status, "code": code, "detail": detail})
}
func (s *Server) Shutdown() {
	s.mu.Lock()
	s.closed = true
	r := s.active
	s.mu.Unlock()
	if r != nil {
		r.Cancel()
		<-r.Done()
	}
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != r.Host {
			apiError(w, 403, "ORIGIN_REJECTED", "Use the same-origin local UI.")
			return
		}
	}
	if r.URL.RawQuery != "" {
		apiError(w, 400, "INVALID_REQUEST", "Query parameters are not supported.")
		return
	}
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]string{"status": "UP"})
		return
	}
	if r.URL.Path == "/v1/runs" && r.Method == http.MethodPost {
		media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || media != "application/json" {
			apiError(w, 415, "INVALID_MEDIA_TYPE", "Use application/json.")
			return
		}
		data, e := io.ReadAll(io.LimitReader(r.Body, 262145))
		if e != nil {
			apiError(w, 400, "INVALID_REQUEST", "Cannot read scenario.")
			return
		}
		if len(data) > 262144 {
			apiError(w, 413, "SCENARIO_TOO_LARGE", "Scenario exceeds 256 KiB.")
			return
		}
		scenario, e := Decode(data)
		if e != nil {
			apiError(w, 422, "INVALID_SCENARIO", e.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			apiError(w, 503, "SHUTTING_DOWN", "Runner is shutting down.")
			return
		}
		if s.active != nil {
			select {
			case <-s.active.Done():
			default:
				apiError(w, 409, "RUN_ACTIVE", "Finish or cancel the current run.")
				return
			}
		}
		run, e := Start(context.WithoutCancel(r.Context()), scenario)
		if e != nil {
			apiError(w, 422, "INVALID_SCENARIO", e.Error())
			return
		}
		s.active = run
		s.runs[run.Snapshot().ID] = run
		s.order = append(s.order, run.Snapshot().ID)
		for len(s.order) > 11 {
			delete(s.runs, s.order[0])
			s.order = s.order[1:]
		}
		w.Header().Set("Location", "/v1/runs/"+run.Snapshot().ID)
		writeJSON(w, 202, run.Snapshot())
		return
	}
	if r.URL.Path == "/v1/runs/active" && r.Method == http.MethodGet {
		s.mu.Lock()
		run := s.active
		s.mu.Unlock()
		if run != nil {
			select {
			case <-run.Done():
			default:
				writeJSON(w, 200, run.Snapshot())
				return
			}
		}
		w.WriteHeader(204)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 3 && len(parts) <= 4 && parts[0] == "v1" && parts[1] == "runs" {
		s.mu.Lock()
		run := s.runs[parts[2]]
		s.mu.Unlock()
		if run == nil {
			apiError(w, 404, "RUN_UNAVAILABLE", "Run expired or the runner restarted. Exported reports remain available.")
			return
		}
		if len(parts) == 3 && r.Method == http.MethodGet {
			writeJSON(w, 200, run.Snapshot())
			return
		}
		if len(parts) == 4 {
			switch parts[3] {
			case "attempts":
				if r.Method == http.MethodGet {
					writeJSON(w, 200, run.Snapshot().Attempts)
					return
				}
			case "report":
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Disposition", "attachment; filename=hookprobe-"+run.Snapshot().ID+".json")
					writeJSON(w, 200, run.Snapshot())
					return
				}
			case "cancel":
				if r.Method == http.MethodPost {
					run.Cancel()
					writeJSON(w, 202, map[string]string{"status": "cancellation_requested"})
					return
				}
			}
		}
		apiError(w, 405, "METHOD_NOT_ALLOWED", "Use the documented run method.")
		return
	}
	apiError(w, 404, "NOT_FOUND", "Unknown runner route.")
}
