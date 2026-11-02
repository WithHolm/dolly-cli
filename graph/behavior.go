package graph

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Behavior is how dolly answers, apart from the data. A person changes it to
// see how a tool reacts to a slow Graph, to throttling, or to a failure.
type Behavior struct {
	// LatencyMS is a wait before each answer.
	LatencyMS int `json:"latencyMs"`
	// FailPercent is the share of requests (0 to 100) that get the error
	// FailStatus, with a Retry-After header for 429 and 503. Each request in a
	// $batch counts on its own, as in Graph.
	FailPercent int `json:"failPercent"`
	FailStatus  int `json:"failStatus"` // 429, 500, 502, 503, or 504
	RetryAfter  int `json:"retryAfter"` // seconds
	// Unauthorized makes every request fail with 401.
	Unauthorized bool `json:"unauthorized"`
	// PageSize forces the size of every page (0 uses $top).
	PageSize int `json:"pageSize"`
	// Throttle makes a request that passes the limits of Graph (see package cost) fail with
	// 429. The cost of each call is counted in any case.
	Throttle bool `json:"throttle"`
	// DuplicatePercent is the share of change notifications (0 to 100) that are sent more than once: 2 to 4 times, with the same message. Graph does this when
	// the services that hold the data are not in step.
	DuplicatePercent int `json:"duplicatePercent"`
}

// copies says how many times the next change notification is sent.
func (b Behavior) copies() int {
	if b.DuplicatePercent > 0 && rand.IntN(100) < b.DuplicatePercent {
		return 2 + rand.IntN(3)
	}
	return 1
}

// check makes the values right: a wrong value is an error, so the person sees it.
func (b Behavior) check() error {
	switch {
	case b.LatencyMS < 0 || b.LatencyMS > 60000:
		return fmt.Errorf("latencyMs must be from 0 to 60000")
	case b.FailPercent < 0 || b.FailPercent > 100:
		return fmt.Errorf("failPercent must be from 0 to 100")
	case b.DuplicatePercent < 0 || b.DuplicatePercent > 100:
		return fmt.Errorf("duplicatePercent must be from 0 to 100")
	case b.RetryAfter < 0 || b.RetryAfter > 300:
		return fmt.Errorf("retryAfter must be from 0 to 300")
	case b.PageSize < 0 || b.PageSize > maxPage:
		return fmt.Errorf("pageSize must be from 0 to %d", maxPage)
	}
	switch b.FailStatus {
	case 0, 429, 500, 502, 503, 504:
	default:
		return fmt.Errorf("failStatus must be 429, 500, 502, 503, or 504")
	}
	return nil
}

func (s *Server) behavior() Behavior {
	s.bmu.RLock()
	defer s.bmu.RUnlock()
	return s.beh
}

// fault applies the behavior to one request. It returns the name of the fault
// when it wrote the answer, and the request must stop.
func (s *Server) fault(w http.ResponseWriter, r *http.Request) string {
	b := s.behavior()
	if b.LatencyMS > 0 {
		select {
		case <-time.After(time.Duration(b.LatencyMS) * time.Millisecond):
		case <-r.Context().Done():
		}
	}
	if b.Unauthorized {
		writeError(w, apiErr(http.StatusUnauthorized, "InvalidAuthenticationToken", "Access token has expired or is not yet valid. (dolly: unauthorized is on)"))
		return "unauthorized"
	}
	if b.FailPercent > 0 && rand.IntN(100) < b.FailPercent {
		status := b.FailStatus
		if status == 0 {
			status = http.StatusTooManyRequests
		}
		code, msg := "generalException", "dolly: a failure that was asked for"
		switch status {
		case http.StatusTooManyRequests:
			code, msg = "TooManyRequests", "Too many requests. (dolly: a failure that was asked for)"
		case http.StatusServiceUnavailable:
			code, msg = "serviceNotAvailable", "The service is not available. (dolly: a failure that was asked for)"
		}
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			w.Header().Set("Retry-After", strconv.Itoa(b.RetryAfter))
		}
		writeError(w, apiErr(status, code, msg))
		return "failed " + strconv.Itoa(status)
	}
	return ""
}
