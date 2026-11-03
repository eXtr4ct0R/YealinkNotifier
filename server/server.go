package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"YealinkNotifier/core"
)

type Server struct {
	app *core.App
	srv *http.Server
}

func New(app *core.App, addr string) *Server {
	mux := http.NewServeMux()

	s := &Server{
		app: app,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
	mux.HandleFunc("/", s.handler)

	app.SetShutdown(s.Shutdown)
	return s
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	path := strings.ReplaceAll(strings.ReplaceAll(r.URL.Path, "\r", "\\r"), "\n", "\\n")
	log.Printf("[%s] Signal received: %s", r.RemoteAddr, path)

	query := r.URL.Query()
	caller := query.Get("number")
	if caller == "" {
		caller = query.Get("caller")
	}

	s.app.OnCall(caller)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) ListenAndServe() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) {
	log.Printf("Stopping server (draining in-flight requests, timeout: 5s)")
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down server: %v", err)
		return
	}
	log.Printf("Server stopped")
}
