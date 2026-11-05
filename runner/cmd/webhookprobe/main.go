package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"webhookprobe/runner/internal/probe"
)

func main() { os.Exit(command()) }
func command() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: webhookprobe serve [-listen 127.0.0.1:8081] | run -scenario file.json [-out report.json]")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch os.Args[1] {
	case "run":
		f := flag.NewFlagSet("run", flag.ContinueOnError)
		file := f.String("scenario", "", "Scenario JSON file")
		out := f.String("out", "", "Report path; default stdout")
		if f.Parse(os.Args[2:]) != nil || *file == "" {
			return 2
		}
		input, e := os.Open(*file)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 2
		}
		defer input.Close()
		data, e := io.ReadAll(io.LimitReader(input, 262145))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 2
		}
		s, e := probe.Decode(data)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 2
		}
		r, e := probe.Start(ctx, s)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 2
		}
		<-r.Done()
		report := r.Snapshot()
		body, _ := json.MarshalIndent(report, "", "  ")
		body = append(body, '\n')
		if *out == "" {
			os.Stdout.Write(body)
		} else if e := os.WriteFile(*out, body, 0600); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 2
		}
		if report.State == "cancelled" {
			return 130
		}
		if report.Verdict == "failed" {
			return 1
		}
		if report.Verdict == "inconclusive" {
			return 3
		}
		return 0
	case "serve":
		f := flag.NewFlagSet("serve", flag.ContinueOnError)
		address := f.String("listen", "127.0.0.1:8081", "HTTP bind address")
		if f.Parse(os.Args[2:]) != nil {
			return 2
		}
		handler := probe.NewServer()
		server := &http.Server{Addr: *address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
		done := make(chan error, 1)
		go func() { done <- server.ListenAndServe() }()
		fmt.Fprintln(os.Stderr, "WebHookProbe runner:", *address)
		select {
		case e := <-done:
			if e != http.ErrServerClosed {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
		case <-ctx.Done():
			handler.Shutdown()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(shutdown)
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "Unknown command")
		return 2
	}
}
