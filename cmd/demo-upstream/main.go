package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	demoAddress       = "127.0.0.1:18080"
	demoClosedAddress = "127.0.0.1:18081"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	closedProbe, err := net.Listen("tcp", demoClosedAddress)
	if err != nil {
		return fmt.Errorf("demo requires %s to be unused: %w", demoClosedAddress, err)
	}
	if err := closedProbe.Close(); err != nil {
		return fmt.Errorf("release demo closed port %s: %w", demoClosedAddress, err)
	}

	listener, err := net.Listen("tcp", demoAddress)
	if err != nil {
		return fmt.Errorf("listen on demo address %s: %w", demoAddress, err)
	}

	server := &http.Server{
		Handler:           newHandler(),
		ReadHeaderTimeout: 2 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "demo upstream listening on http://%s\n", demoAddress)
	fmt.Fprintln(os.Stderr, "  GET /ready     -> 200")
	fmt.Fprintln(os.Stderr, "  GET /unhealthy -> 503")

	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		return nil
	}
	return fmt.Errorf("serve demo upstream: %w", err)
}

func newHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/unhealthy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	return mux
}
