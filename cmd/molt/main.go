// Command molt is the MoltNet CLI: create identities, register agents, issue
// attestations, and — the flagship — verify an agent's entire history offline,
// trusting no registry.
package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprint(os.Stderr, `molt — MoltNet CLI

USAGE:
  molt <command> [flags]

COMMANDS:
  keygen     Create an owner or agent keypair
  card       Build and sign an agent card (subcommand: new)
  register   Sign-check and submit a card to a registry
  attest     Issue a signed attestation about an agent
  verify     Fetch an agent's chain, verify signatures, recompute score locally
  search     Search the registry by text, capability and min score
  badge      Print a Markdown badge snippet for an agent
  serve      Run a local moltnetd instance (single-node quickstart)
  mcp        Run an MCP (Model Context Protocol) server over stdio for agents

Run "molt <command> -h" for command flags.
Registry defaults to $MOLTNET_REGISTRY or http://localhost:8787.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	case "card":
		err = cmdCard(os.Args[2:])
	case "register":
		err = cmdRegister(os.Args[2:])
	case "attest":
		err = cmdAttest(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "search":
		err = cmdSearch(os.Args[2:])
	case "badge":
		err = cmdBadge(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "mcp":
		err = cmdMCP(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
