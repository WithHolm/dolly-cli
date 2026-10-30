package graph

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"
)

// Call is one request that dolly got, with its answer. The page shows the
// list, so a person can see what a tool asked for.
type Call struct {
	// ID names the call. A call inside a $batch has the ID of the $batch in Batch.
	ID    int64 `json:"id"`
	Batch int64 `json:"batch,omitempty"`
	Index int   `json:"index,omitempty"` // the place inside the $batch

	// Seq grows each time a call is done. A page asks for the calls with a
	// larger Seq than the last one it has. (A $batch is done after its parts,
	// so ID cannot be the cursor.)
	Seq int64 `json:"seq"`

	Time        time.Time `json:"time"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Query       string    `json:"query,omitempty"`
	Status      int       `json:"status"`
	Millis      float64   `json:"ms"`
	RU          int       `json:"ru,omitempty"` // the cost of the call, in resource units
	Bytes       int       `json:"bytes"`
	Consistency string    `json:"consistency,omitempty"` // the ConsistencyLevel header
	Agent       string    `json:"agent,omitempty"`
	Fault       string    `json:"fault,omitempty"` // why dolly changed the answer
	Request     string    `json:"request,omitempty"`
	Response    string    `json:"response,omitempty"`
}

const (
	// maxCalls is how many calls dolly keeps. The oldest go first.
	maxCalls = 2000
	// maxBody is how much of a request or answer body dolly keeps for the page.
	maxBody = 4096
)

// callLog is the list of the last calls.
type callLog struct {
	mu     sync.Mutex
	nextID int64
	seq    int64
	calls  []Call
}

// reserve gives the ID of a call that starts now.
func (l *callLog) reserve() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	return l.nextID
}

// add saves a call that is done.
func (l *callLog) add(c Call) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	c.Seq = l.seq
	l.calls = append(l.calls, c)
	if len(l.calls) > maxCalls {
		l.calls = append([]Call(nil), l.calls[len(l.calls)-maxCalls:]...)
	}
}

// logCall saves a call that is done, unless the client of the request has the flag hideCalls.
// r is the request of the client (for a $batch, the outer one: it has the token).
func (s *Server) logCall(r *http.Request, c Call) {
	if _, hide := s.world.ClientFlags(s.appID(r)); !hide {
		s.calls.add(c)
	}
}

// since gives the calls that are done after the cursor, oldest first, and the
// new cursor. Limit is the most calls to give (0 is all of them).
func (l *callLog) since(cursor int64, limit int) ([]Call, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Call
	for _, c := range l.calls {
		if c.Seq > cursor {
			out = append(out, c)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, l.seq
}

func (l *callLog) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = nil
}

// recorder keeps what a handler wrote: the status, the size, and the start of the body.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
	head   bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if room := maxBody - r.head.Len(); room > 0 {
		r.head.Write(p[:min(len(p), room)])
	}
	r.bytes += len(p)
	return r.ResponseWriter.Write(p)
}

// readBody reads the body of a request for the log, and puts it back for the handler.
func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	if len(data) > maxBody {
		return string(data[:maxBody]) + "…"
	}
	return string(data)
}

func cut(s string) string {
	if len(s) >= maxBody {
		return s + "…"
	}
	return s
}
