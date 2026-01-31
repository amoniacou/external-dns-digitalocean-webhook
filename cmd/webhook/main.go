package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/amoniacou/external-dns-digitalocean-webhook/internal/digitalocean"
	"github.com/amoniacou/external-dns-digitalocean-webhook/internal/server"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Parse flags
	var (
		logLevel    string
		logFormat   string
		host        string
		port        int
		dryRun      bool
		retryMax    int
		retryWaitMax time.Duration
	)

	flag.StringVar(&logLevel, "log-level", getEnvOrDefault("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.StringVar(&logFormat, "log-format", getEnvOrDefault("LOG_FORMAT", "text"), "Log format (text, json)")
	flag.StringVar(&host, "host", getEnvOrDefault("WEBHOOK_HOST", "0.0.0.0"), "Webhook server host")
	flag.IntVar(&port, "port", getEnvOrDefaultInt("WEBHOOK_PORT", 8888), "Webhook server port")
	flag.BoolVar(&dryRun, "dry-run", getEnvOrDefaultBool("DO_DRY_RUN", false), "Dry run mode")
	flag.IntVar(&retryMax, "retry-max", getEnvOrDefaultInt("DO_HTTP_RETRY_MAX", 3), "Maximum HTTP retries")
	flag.DurationVar(&retryWaitMax, "retry-wait-max", getEnvOrDefaultDuration("DO_HTTP_RETRY_WAIT_MAX", 30*time.Second), "Maximum wait between retries")
	flag.Parse()

	// Setup logging
	setupLogging(logLevel, logFormat)

	log.Infof("Starting external-dns-digitalocean-webhook version=%s commit=%s date=%s", version, commit, date)

	// Load configuration
	cfg, err := digitalocean.NewConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override with flags
	cfg.DryRun = dryRun
	cfg.HTTPRetryMax = retryMax
	cfg.HTTPRetryWaitMax = retryWaitMax

	// Create provider
	provider, err := digitalocean.NewProvider(cfg)
	if err != nil {
		log.Fatalf("Failed to create DigitalOcean provider: %v", err)
	}

	// Create server
	srv := server.New(provider, &server.Config{
		Host:         host,
		Port:         port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Infof("Received signal %s, shutting down...", sig)
		cancel()
	}()

	// Start server
	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	log.Info("Shutdown complete")
}

func setupLogging(level, format string) {
	switch level {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	if format == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		if _, err := parseIntFromString(v, &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvOrDefaultBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1"
	}
	return defaultValue
}

func getEnvOrDefaultDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func parseIntFromString(s string, i *int) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	*i = n
	return n, nil
}
