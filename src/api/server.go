package api

import (
	"encoding/json"
	"net/http"
	scaler "nextcast/src/core"
	nexhistory "nextcast/src/history"
	"nextcast/src/logx"
	"time"
)

type Handler interface {
	SelfAddr() string
	ClusterToken() string
	NodeInfo() scaler.NodeInfoResponse
	ServicesState() (scaler.ServicesStateResponse, error)
	History() (nexhistory.Response, error)
	HandleScaleCommand(request scaler.ScaleCommandRequest) (scaler.ScaleCommandResponse, int, error)
}

type Server struct {
	handler Handler
}

func NewServer(handler Handler) *Server {
	return &Server{handler: handler}
}

func (s *Server) authorize(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+s.handler.ClusterToken()
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authorize(r) {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func applyCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
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
	if !s.requireAuth(w, r) {
		return
	}
	writeJSON(w, s.handler.NodeInfo())
}

func (s *Server) handleServicesState(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	state, err := s.handler.ServicesState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, state)
}

func (s *Server) handleScaleCommand(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	var request scaler.ScaleCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response, statusCode, err := s.handler.HandleScaleCommand(request)
	if err != nil {
		http.Error(w, err.Error(), statusCode)
		return
	}

	writeJSON(w, response)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

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
	mux.HandleFunc("/scaleCommand", s.withCORS(s.handleScaleCommand))

	server := &http.Server{
		Addr:              s.handler.SelfAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logx.Infof("cluster API listening on %s", s.handler.SelfAddr())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Fatalf("cluster API failed: %v", err)
		}
	}()
}
