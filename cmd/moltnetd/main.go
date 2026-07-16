// Command moltnetd is the MoltNet registry server: a single self-hostable
// binary that stores agent cards and attestations, verifies signatures and
// chain integrity on ingest, serves discovery and badges, and (optionally)
// serves the web UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moltnet/moltnet/internal/server"
	"github.com/moltnet/moltnet/internal/store"
)

const version = "0.1.0"

// peerList collects repeated --peer flags.
type peerList []string

func (p *peerList) String() string { return fmt.Sprintf("%v", []string(*p)) }
func (p *peerList) Set(v string) error {
	*p = append(*p, v)
	return nil
}

// envOr returns the environment variable k, or def if it is unset/empty. Flags
// still win — this only changes the default, so a container platform can
// configure the instance (which usually can only set env, not argv) while the
// CLI keeps working unchanged.
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	var (
		addr   = flag.String("addr", ":8787", "listen address")
		dbPath = flag.String("db", "moltnet.db", "SQLite database path (or :memory:)")
		appDir = flag.String("app", "", "built React SPA to serve at / (e.g. frontend/dist)")
		name   = flag.String("name", envOr("MOLTNET_NAME", "moltnet local instance"), "instance name in /.well-known/moltnet ($MOLTNET_NAME)")
		probe  = flag.Duration("probe-interval", 5*time.Minute, "liveness probe sweep interval (0 disables)")
		fedInt = flag.Duration("federation-interval", 30*time.Second, "federation pull interval (0 disables)")
		// Non-zero by default: POST /v1/auth/challenge is unauthenticated and
		// writes a row per call, so an open instance should not ship with every
		// write path unthrottled. Generous enough that honest clients (register,
		// attest, sign-in) never notice; set 0 to disable explicitly.
		rlimit = flag.Int("rate-limit", 120, "max write requests per client IP per minute (0 disables)")
		logReq = flag.Bool("log-requests", false, "write one structured JSON log line per request to stderr")
	)
	var peers peerList
	flag.Var(&peers, "peer", "federation peer base URL to follow (repeatable)")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := &server.Server{Store: st, AppDir: *appDir, Name: *name, Version: version, Peers: peers, RateLimitPerMin: *rlimit}
	if *logReq {
		srv.LogWriter = os.Stderr
	}
	srv.StartLivenessProber(*probe)
	srv.StartFederation(*fedInt)
	// Reap spent SIWK challenges and expired sessions. /v1/auth/challenge is
	// unauthenticated, so without this the auth tables grow without bound.
	srv.StartAuthGC(time.Hour)

	fmt.Fprintf(os.Stderr, "moltnetd %s\n", version)
	fmt.Fprintf(os.Stderr, "  db:   %s\n", *dbPath)
	if *appDir != "" {
		fmt.Fprintf(os.Stderr, "  app:  %s\n", *appDir)
	}
	fmt.Fprintf(os.Stderr, "  listening on http://localhost%s\n", *addr)

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler()}

	// Serve until a termination signal, then shut down gracefully so in-flight
	// requests finish and the SQLite WAL is checkpointed cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		log.Fatalf("server: %v", err)
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "\nshutting down gracefully…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("graceful shutdown failed: %v", err)
		}
		fmt.Fprintln(os.Stderr, "stopped")
	}
}
