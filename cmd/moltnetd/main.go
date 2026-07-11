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

func main() {
	var (
		addr   = flag.String("addr", ":8787", "listen address")
		dbPath = flag.String("db", "moltnet.db", "SQLite database path (or :memory:)")
		webDir = flag.String("web", "", "directory of static web assets to serve at / (optional)")
		appDir = flag.String("app", "", "built SPA directory to serve at / (e.g. frontend/dist, optional)")
		name   = flag.String("name", "moltnet local instance", "instance name in /.well-known/moltnet")
		probe  = flag.Duration("probe-interval", 5*time.Minute, "liveness probe sweep interval (0 disables)")
		fedInt = flag.Duration("federation-interval", 30*time.Second, "federation pull interval (0 disables)")
		rlimit = flag.Int("rate-limit", 0, "max write requests per client IP per minute (0 disables)")
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

	srv := &server.Server{Store: st, WebDir: *webDir, AppDir: *appDir, Name: *name, Version: version, Peers: peers, RateLimitPerMin: *rlimit}
	if *logReq {
		srv.LogWriter = os.Stderr
	}
	srv.StartLivenessProber(*probe)
	srv.StartFederation(*fedInt)

	fmt.Fprintf(os.Stderr, "moltnetd %s\n", version)
	fmt.Fprintf(os.Stderr, "  db:   %s\n", *dbPath)
	if *webDir != "" {
		fmt.Fprintf(os.Stderr, "  web:  %s\n", *webDir)
	}
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
