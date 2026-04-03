package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	shutdownGracePeriod := 10 * time.Second
	if raw := os.Getenv("SHUTDOWN_GRACE_PERIOD"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			log.Fatalf("invalid SHUTDOWN_GRACE_PERIOD %q: %v", raw, err)
		}
		shutdownGracePeriod = parsed
	}

	var concurrentConnections int64
	var tracked sync.Map
	var shuttingDown atomic.Bool

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "nextcast example http server")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if shuttingDown.Load() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ready")
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: nil,
		ConnState: func(c net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				if _, loaded := tracked.LoadOrStore(c, true); !loaded {
					current := atomic.AddInt64(&concurrentConnections, 1)
					log.Printf("connection opened from %s | concurrent=%d", c.RemoteAddr().String(), current)
				}
			case http.StateClosed, http.StateHijacked:
				if _, ok := tracked.Load(c); ok {
					tracked.Delete(c)
					current := atomic.AddInt64(&concurrentConnections, -1)
					log.Printf("connection closed from %s | concurrent=%d", c.RemoteAddr().String(), current)
				}
			}
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shuttingDown.Store(true)
		log.Printf("shutdown signal received, draining for %s", shutdownGracePeriod)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("example server listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
