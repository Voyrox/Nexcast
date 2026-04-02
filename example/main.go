package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var concurrentConnections int64
	var tracked sync.Map

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "nextcast example http server")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
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

	log.Printf("example server listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}
