package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the status code (which
// defaults to 200 if WriteHeader is never called).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req"
	}
	return hex.EncodeToString(b[:])
}

// withLogging assigns/propagates a request id (X-Request-Id) and writes one
// structured JSON log line per request to w. A nil writer disables logging.
func withLogging(w io.Writer, h http.Handler) http.Handler {
	if w == nil {
		return h
	}
	enc := json.NewEncoder(w)
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		rw.Header().Set("X-Request-Id", id)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: rw, status: 200}
		h.ServeHTTP(rec, r)

		_ = enc.Encode(map[string]any{
			"time":   start.UTC().Format(time.RFC3339),
			"id":     id,
			"method": r.Method,
			"path":   r.URL.Path,
			"status": rec.status,
			"dur_ms": time.Since(start).Milliseconds(),
			"ip":     clientIP(r),
		})
	})
}
