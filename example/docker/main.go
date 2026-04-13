package main

import (
	"context"
	"encoding/json"
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

type rollingCounter struct {
	mu      sync.Mutex
	buckets [60]int64
	seconds [60]int64
	started time.Time
}

func newRollingCounter() *rollingCounter {
	return &rollingCounter{started: time.Now().UTC()}
}

func (r *rollingCounter) Record(ts time.Time) {
	second := ts.UTC().Unix()
	idx := int(second % int64(len(r.buckets)))
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seconds[idx] != second {
		r.seconds[idx] = second
		r.buckets[idx] = 0
	}
	r.buckets[idx]++
}

func (r *rollingCounter) RPS(now time.Time) float64 {
	nowSecond := now.UTC().Unix()
	cutoff := nowSecond - int64(len(r.buckets)-1)
	r.mu.Lock()
	defer r.mu.Unlock()
	count := int64(0)
	for i, second := range r.seconds {
		if second >= cutoff && second <= nowSecond {
			count += r.buckets[i]
		}
	}
	elapsed := now.UTC().Sub(r.started).Seconds()
	window := float64(len(r.buckets))
	if elapsed > 0 && elapsed < window {
		window = elapsed
	}
	if window <= 0 {
		window = 1
	}
	return float64(count) / window
}

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
	var totalRequests atomic.Int64
	traffic := newRollingCounter()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "nextcast example http server")
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if shuttingDown.Load() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ready")
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rps":                    traffic.RPS(time.Now().UTC()),
			"concurrent_connections": atomic.LoadInt64(&concurrentConnections),
			"total_requests":         totalRequests.Load(),
		})
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" && r.URL.Path != "/health" && r.URL.Path != "/ready" {
			traffic.Record(time.Now().UTC())
			totalRequests.Add(1)
		}
		mux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
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
