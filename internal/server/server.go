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
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		Host:         "0.0.0.0",
		Port:         8888,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// Server is the webhook HTTP server
type Server struct {
	provider provider.Provider
	config   *Config
	server   *http.Server
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

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/healthz", s.healthHandler)

	// Metrics
	mux.Handle("/metrics", promhttp.Handler())

	// Webhook API endpoints
	mux.HandleFunc("/", s.negotiateHandler)
	mux.HandleFunc("/records", s.recordsHandler)
	mux.HandleFunc("/adjustendpoints", s.adjustEndpointsHandler)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.Infof("Starting webhook server on %s", addr)

	go func() {
		<-ctx.Done()
		log.Info("Shutting down webhook server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("Error shutting down server")
		}
	}()

	if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
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
	w.Write([]byte("OK"))
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
