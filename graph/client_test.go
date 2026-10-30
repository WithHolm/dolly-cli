package graph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// client is a small Graph client for the tests. It has no code from other
// modules. It tries a call again when the answer is 429 or 5xx, as a real tool does.
type client struct {
	BaseURL string
	NoToken bool
}

// graphError is an answer with a status that is not a success.
type graphError struct{ Status int }

func (e *graphError) Error() string { return fmt.Sprintf("Graph returned status %d", e.Status) }

type batchRequest struct {
	ID, Method, URL string
	Body            any
}

type batchResponse struct {
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

const tries = 3

func (c *client) do(ctx context.Context, method, target string, body any, header http.Header) ([]byte, error) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	var err error
	for range tries {
		var req *http.Request
		if req, err = http.NewRequestWithContext(ctx, method, c.BaseURL+target, bytes.NewReader(payload)); err != nil {
			return nil, err
		}
		if !c.NoToken {
			req.Header.Set("Authorization", "Bearer any")
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range header {
			req.Header[k] = v
		}
		var res *http.Response
		if res, err = http.DefaultClient.Do(req); err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode < 300 {
			return data, nil
		}
		err = &graphError{Status: res.StatusCode}
		if res.StatusCode != 429 && res.StatusCode < 500 {
			return nil, err
		}
	}
	return nil, err
}

func (c *client) Get(ctx context.Context, target string, out any) error {
	return c.GetWith(ctx, target, nil, out)
}

func (c *client) GetWith(ctx context.Context, target string, header http.Header, out any) error {
	data, err := c.do(ctx, http.MethodGet, target, nil, header)
	if err != nil || out == nil || len(data) == 0 {
		return err
	}
	return json.Unmarshal(data, out)
}

// Batch sends the requests in one $batch call. A part that fails is not an error of the call.
func (c *client) Batch(ctx context.Context, reqs []batchRequest) ([]batchResponse, error) {
	items := make([]map[string]any, len(reqs))
	for i, r := range reqs {
		items[i] = map[string]any{"id": r.ID, "method": r.Method, "url": r.URL, "body": r.Body}
		if r.Body != nil {
			items[i]["headers"] = map[string]string{"Content-Type": "application/json"}
		}
	}
	data, err := c.do(ctx, http.MethodPost, "/$batch", map[string]any{"requests": items}, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Responses []batchResponse `json:"responses"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out.Responses, nil
}
