package api

import (
	"encoding/json"
	"net/http"
	nextcast "nextcast/src/core"
	"nextcast/src/history"
	"nextcast/src/logx"
	"time"
)

type Handler interface {
	SelfAddr() string
	NodeInfo() nextcast.NodeInfoResponse
	ServicesState() (nextcast.ServicesStateResponse, error)
	History() (history.Response, error)
}

type Server struct {
	handler Handler
}

func NewServer(handler Handler) *Server {
	return &Server{handler: handler}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func applyCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (s *Server) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applyCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.handler.NodeInfo())
}

func (s *Server) handleServicesState(w http.ResponseWriter, r *http.Request) {
	state, err := s.handler.ServicesState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, state)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	historyResponse, err := s.handler.History()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, historyResponse)
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/nodeInfo", s.withCORS(s.handleNodeInfo))
	mux.HandleFunc("/servicesState", s.withCORS(s.handleServicesState))
	mux.HandleFunc("/history", s.withCORS(s.handleHistory))

	server := &http.Server{
		Addr:              s.handler.SelfAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logx.Infof("API listening on %s", s.handler.SelfAddr())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Fatalf("API failed: %v", err)
		}
	}()
}
