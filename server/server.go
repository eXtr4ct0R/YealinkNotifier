package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"YealinkNotifier/core"
)

type Server struct {
	app  *core.App
	srv  *http.Server
	nets []*net.IPNet
}

func New(app *core.App, addr string, allowedIPs []string) (*Server, error) {
	nets, err := parseAllowedIPs(allowedIPs)
	if err != nil {
		return nil, err
	}

	s := &Server{
		app:  app,
		nets: nets,
		srv: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handler)
	s.srv.Handler = mux

	app.SetShutdown(s.Shutdown)
	return s, nil
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowed(ip) {
		log.Printf("rejected webhook from %s: not in allowed-ips", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

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

func (s *Server) allowed(ip net.IP) bool {
	if s.nets == nil {
		return true
	}
	return ipAllowed(s.nets, ip)
}

func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func parseAllowedIPs(values []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(v); err == nil {
			nets = append(nets, ipNet)
			continue
		}
		if ip := net.ParseIP(v); ip != nil {
			nets = append(nets, singleHost(ip))
			continue
		}
		return nil, fmt.Errorf("%q is not a valid IP or CIDR entry", v)
	}
	return nets, nil
}

func singleHost(ip net.IP) *net.IPNet {
	bits := 128
	if ip.To4() != nil {
		bits = 32
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
}

func ipAllowed(nets []*net.IPNet, ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
