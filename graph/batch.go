package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/withholm/dolly-cli/graph/api"
)

// maxBatch is the most requests in one $batch call, as in Graph.
const maxBatch = 20

type batchRequest struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// batch runs each request of a $batch call through the same handlers as a
// request of its own, and puts the answers together. It is part of the server and
// not of the api package, because it runs the resources there, and it must keep
// the list of calls, the faults, and the counts for each part.
func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Requests []batchRequest `json:"requests"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, badRequest("The $batch body is not valid JSON."))
		return
	}
	if len(in.Requests) == 0 || len(in.Requests) > maxBatch {
		writeError(w, badRequest("A $batch call needs 1 to 20 requests."))
		return
	}
	parent, _ := r.Context().Value(callIDKey{}).(int64)
	responses := make([]map[string]any, 0, len(in.Requests))
	for i, br := range in.Requests {
		var body io.Reader
		if len(br.Body) > 0 && string(br.Body) != "null" {
			body = bytes.NewReader(br.Body)
		}
		inner := httptest.NewRequest(strings.ToUpper(br.Method), "http://"+r.Host+BasePath+br.URL, body)
		inner.Header.Set("Authorization", r.Header.Get("Authorization"))
		for k, v := range br.Headers {
			inner.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		api.Stamp(rec.Header(), inner) // each request has its own ids, as in Graph
		start := time.Now()
		call := Call{
			ID: s.calls.reserve(), Batch: parent, Index: i, Time: start, Method: inner.Method, Path: inner.URL.Path, Query: inner.URL.RawQuery,
			Consistency: inner.Header.Get("ConsistencyLevel"), Agent: r.UserAgent(), Request: string(br.Body),
		}
		// a change inside the $batch names the tool, and the call that made it
		inner.Header.Set("User-Agent", r.UserAgent())
		inner = inner.WithContext(context.WithValue(inner.Context(), callIDKey{}, call.ID))
		// each request in a $batch can fail on its own
		if me, e := s.meRequest(inner); e != nil {
			writeError(rec, e)
			call.Fault = "me"
		} else {
			inner = me
			call.Fault = s.fault(rec, inner)
		}
		if call.Fault == "" {
			var throttled bool
			if call.RU, throttled = s.charge(rec.Header(), inner); throttled {
				call.Fault = "throttled"
				writeError(rec, apiErr(http.StatusTooManyRequests, "TooManyRequests", "Too many requests. (dolly: the limit of Graph for this client)"))
			} else {
				s.mux.ServeHTTP(rec, inner)
				s.track(inner, rec.Code)
			}
		}
		call.Status, call.Bytes, call.Response = rec.Code, rec.Body.Len(), cut(rec.Body.String()[:min(rec.Body.Len(), maxBody)])
		call.Millis = float64(time.Since(start).Microseconds()) / 1000
		s.logCall(r, call)

		headers := map[string]string{"Content-Type": rec.Header().Get("Content-Type")}
		for _, h := range []string{"Retry-After", "x-ms-resource-unit", "x-ms-throttle-limit-percentage", "x-ms-throttle-scope", "x-ms-throttle-information"} {
			if v := rec.Header().Get(h); v != "" {
				headers[h] = v
			}
		}
		resp := map[string]any{"id": br.ID, "status": rec.Code, "headers": headers}
		if raw := bytes.TrimSpace(rec.Body.Bytes()); len(raw) > 0 {
			switch {
			case strings.Contains(rec.Header().Get("Content-Type"), "json"):
				resp["body"] = json.RawMessage(raw)
			default:
				if _, err := strconv.Atoi(string(raw)); err == nil {
					resp["body"] = json.RawMessage(raw) // a $count answer is a number
				} else {
					resp["body"] = string(raw)
				}
			}
		}
		responses = append(responses, resp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"responses": responses})
}
