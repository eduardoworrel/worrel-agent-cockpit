package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/hookprompt"
)

// runHookCmd trata "worrel hook <sub> ...". Hoje só "prompt".
func runHookCmd(args []string) {
	if len(args) == 0 || args[0] != "prompt" {
		fmt.Fprintln(os.Stderr, "uso: worrel hook prompt --session <id> --port <port> [--format claude|codex]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("hook prompt", flag.ExitOnError)
	session := fs.String("session", "", "id da sessão")
	port := fs.Int("port", 0, "porta do worrel")
	format := fs.String("format", "claude", "formato de saída: claude|codex")
	_ = fs.Parse(args[1:])

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", *port)
	if err := hookprompt.Run(os.Stdin, os.Stdout, baseURL, *session, *format); err != nil {
		fmt.Fprintln(os.Stderr, "hook prompt:", err)
	}
}
