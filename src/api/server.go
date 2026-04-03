package api

import (
	"encoding/json"
	"log"
	"net/http"
	"nextcast/src/scaler"
	"time"
)

type Handler interface {
	SelfAddr() string
	ClusterToken() string
	NodeInfo() scaler.NodeInfoResponse
	ServicesState() (scaler.ServicesStateResponse, error)
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

func (s *Server) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.handler.NodeInfo())
}

func (s *Server) handleServicesState(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := s.handler.ServicesState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *Server) handleScaleCommand(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/nodeInfo", s.handleNodeInfo)
	mux.HandleFunc("/servicesState", s.handleServicesState)
	mux.HandleFunc("/scaleCommand", s.handleScaleCommand)

	server := &http.Server{
		Addr:              s.handler.SelfAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("cluster API listening on %s", s.handler.SelfAddr())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("cluster API failed: %v", err)
		}
	}()
}
