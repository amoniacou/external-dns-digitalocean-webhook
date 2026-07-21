package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	mediaTypeVersion  = "application/external.dns.webhook+json;version=1"
	contentTypeHeader = "Content-Type"
)

// Config holds server configuration
type Config struct {
	Host         string
	Port         int
	HealthHost   string
	HealthPort   int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		Host:         "127.0.0.1",
		Port:         8080,
		HealthHost:   "0.0.0.0",
		HealthPort:   8888,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// Server is the webhook HTTP server
type Server struct {
	provider     provider.Provider
	config       *Config
	server       *http.Server
	healthServer *http.Server
}

// New creates a new webhook server
func New(p provider.Provider, cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Server{
		provider: p,
		config:   cfg,
	}
}

// buildWebhookMux returns the mux serving the private webhook API endpoints.
func (s *Server) buildWebhookMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.negotiateHandler)
	mux.HandleFunc("/records", s.recordsHandler)
	mux.HandleFunc("/adjustendpoints", s.adjustEndpointsHandler)
	return mux
}

// buildHealthMux returns the mux serving the public health and metrics endpoints.
func (s *Server) buildHealthMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthHandler)
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// Start starts the webhook and health HTTP servers
func (s *Server) Start(ctx context.Context) error {
	webhookAddr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	healthAddr := fmt.Sprintf("%s:%d", s.config.HealthHost, s.config.HealthPort)

	s.server = &http.Server{
		Addr:         webhookAddr,
		Handler:      s.loggingMiddleware(s.buildWebhookMux()),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}
	s.healthServer = &http.Server{
		Addr:         healthAddr,
		Handler:      s.loggingMiddleware(s.buildHealthMux()),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	webhookListener, err := net.Listen("tcp", webhookAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", webhookAddr, err)
	}
	healthListener, err := net.Listen("tcp", healthAddr)
	if err != nil {
		_ = webhookListener.Close()
		return fmt.Errorf("failed to listen on %s: %w", healthAddr, err)
	}

	log.Infof("Starting webhook API server on %s", webhookAddr)
	log.Infof("Starting health/metrics server on %s", healthAddr)

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-serveCtx.Done()
		log.Info("Shutting down servers...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("Error shutting down webhook server")
		}
		if err := s.healthServer.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("Error shutting down health server")
		}
	}()

	errCh := make(chan error, 2)
	serve := func(srv *http.Server, listener net.Listener, name string) {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("%s server error: %w", name, err)
			cancel()
			return
		}
		errCh <- nil
	}

	go serve(s.server, webhookListener, "webhook")
	go serve(s.healthServer, healthListener, "health")

	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.WithFields(log.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   wrapped.statusCode,
			"duration": time.Since(start).String(),
		}).Debug("HTTP request")
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) negotiateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(contentTypeHeader, mediaTypeVersion)
	if err := json.NewEncoder(w).Encode(s.provider.GetDomainFilter()); err != nil {
		log.WithError(err).Error("Failed to encode domain filter")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) recordsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getRecords(w, r)
	case http.MethodPost:
		s.applyChanges(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.provider.Records(r.Context())
	if err != nil {
		log.WithError(err).Error("Failed to get records")
		http.Error(w, "Failed to get records", http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, mediaTypeVersion)
	if err := json.NewEncoder(w).Encode(records); err != nil {
		log.WithError(err).Error("Failed to encode records")
	}
}

func (s *Server) applyChanges(w http.ResponseWriter, r *http.Request) {
	var changes plan.Changes
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		log.WithError(err).Error("Failed to decode changes")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.WithFields(log.Fields{
		"creates": len(changes.Create),
		"updates": len(changes.UpdateNew),
		"deletes": len(changes.Delete),
	}).Info("Applying changes")

	if err := s.provider.ApplyChanges(r.Context(), &changes); err != nil {
		log.WithError(err).Error("Failed to apply changes")
		
		// Check if it's a soft error (partial failure)
		// In external-dns, soft errors implement the following interface
		type softError interface {
			SoftError() bool
		}
		
		var se softError
		if errors.As(err, &se) && se.SoftError() {
			// Return 200 for soft errors so external-dns continues its cycle
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Failed to apply changes", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adjustEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var endpoints []*endpoint.Endpoint
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		log.WithError(err).Error("Failed to decode endpoints")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	adjusted, err := s.provider.AdjustEndpoints(endpoints)
	if err != nil {
		log.WithError(err).Error("Failed to adjust endpoints")
		http.Error(w, "Failed to adjust endpoints", http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, mediaTypeVersion)
	if err := json.NewEncoder(w).Encode(adjusted); err != nil {
		log.WithError(err).Error("Failed to encode adjusted endpoints")
	}
}
