// Command moltnetd is the MoltNet registry server: a single self-hostable
// binary that stores agent cards and attestations, verifies signatures and
// chain integrity on ingest, serves discovery and badges, and (optionally)
// serves the web UI.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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
		name   = flag.String("name", "moltnet local instance", "instance name in /.well-known/moltnet")
		probe  = flag.Duration("probe-interval", 5*time.Minute, "liveness probe sweep interval (0 disables)")
		fedInt = flag.Duration("federation-interval", 30*time.Second, "federation pull interval (0 disables)")
	)
	var peers peerList
	flag.Var(&peers, "peer", "federation peer base URL to follow (repeatable)")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := &server.Server{Store: st, WebDir: *webDir, Name: *name, Version: version, Peers: peers}
	srv.StartLivenessProber(*probe)
	srv.StartFederation(*fedInt)

	fmt.Fprintf(os.Stderr, "moltnetd %s\n", version)
	fmt.Fprintf(os.Stderr, "  db:   %s\n", *dbPath)
	if *webDir != "" {
		fmt.Fprintf(os.Stderr, "  web:  %s\n", *webDir)
	}
	fmt.Fprintf(os.Stderr, "  listening on http://localhost%s\n", *addr)

	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
