package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moltnet/moltnet/internal/store"
)

func TestRequestLoggingAndRequestID(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	var buf bytes.Buffer
	srv := &Server{Store: st, Name: "t", Version: "t", LogWriter: &buf}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// A generated request id should appear in the response header.
	resp, err := http.Get(ts.URL + "/v1/taxonomy")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Request-Id") == "" {
		t.Fatal("expected an X-Request-Id response header")
	}

	// The structured log line should record method/path/status.
	var entry struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
		DurMs  any    `json:"dur_ms"`
	}
	line := bytes.TrimSpace(buf.Bytes())
	if err := json.Unmarshal(firstLine(line), &entry); err != nil {
		t.Fatalf("log line is not JSON: %q: %v", line, err)
	}
	if entry.Method != "GET" || entry.Path != "/v1/taxonomy" || entry.Status != 200 || entry.ID == "" {
		t.Fatalf("unexpected log entry: %+v", entry)
	}

	// A caller-supplied request id must be echoed back, not replaced.
	req, _ := http.NewRequest("GET", ts.URL+"/healthz", nil)
	req.Header.Set("X-Request-Id", "caller-supplied-123")
	resp2, _ := http.DefaultClient.Do(req)
	resp2.Body.Close()
	if got := resp2.Header.Get("X-Request-Id"); got != "caller-supplied-123" {
		t.Fatalf("expected echoed request id, got %q", got)
	}
}

func firstLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i]
	}
	return b
}
