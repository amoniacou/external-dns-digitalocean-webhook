package digitalocean

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the DigitalOcean provider
type Config struct {
	// APIToken is the DigitalOcean API token (required)
	APIToken string
	// DomainFilter filters which domains to manage
	DomainFilter []string
	// DryRun enables dry-run mode (no actual changes)
	DryRun bool
	// APIPageSize is the page size for paginated API requests
	APIPageSize int
	// HTTPRetryMax is the maximum number of retries for failed requests
	HTTPRetryMax int
	// HTTPRetryWaitMin is the minimum wait time between retries
	HTTPRetryWaitMin time.Duration
	// HTTPRetryWaitMax is the maximum wait time between retries
	HTTPRetryWaitMax time.Duration
	// Workers is the number of concurrent workers for fetching records
	Workers int
}

// DefaultConfig returns configuration with default values
func DefaultConfig() *Config {
	return &Config{
		APIPageSize:      200,
		HTTPRetryMax:     3,
		HTTPRetryWaitMin: 1 * time.Second,
		HTTPRetryWaitMax: 30 * time.Second,
		Workers:          10,
	}
}

// NewConfigFromEnv creates configuration from environment variables
func NewConfigFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	// Required: API Token
	cfg.APIToken = os.Getenv("DO_TOKEN")
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("DO_TOKEN environment variable is required")
	}

	// Optional: Domain filter (comma-separated)
	if filter := os.Getenv("DO_DOMAIN_FILTER"); filter != "" {
		cfg.DomainFilter = splitAndTrim(filter, ",")
	}

	// Optional: Dry run
	if dryRun := os.Getenv("DO_DRY_RUN"); dryRun != "" {
		cfg.DryRun = dryRun == "true" || dryRun == "1"
	}

	// Optional: API page size
	if pageSize := os.Getenv("DO_API_PAGE_SIZE"); pageSize != "" {
		if v, err := strconv.Atoi(pageSize); err == nil && v > 0 {
			cfg.APIPageSize = v
		}
	}

	// Optional: HTTP retry max
	if retryMax := os.Getenv("DO_HTTP_RETRY_MAX"); retryMax != "" {
		if v, err := strconv.Atoi(retryMax); err == nil && v >= 0 {
			cfg.HTTPRetryMax = v
		}
	}

	// Optional: HTTP retry wait min
	if retryWaitMin := os.Getenv("DO_HTTP_RETRY_WAIT_MIN"); retryWaitMin != "" {
		if v, err := time.ParseDuration(retryWaitMin); err == nil && v > 0 {
			cfg.HTTPRetryWaitMin = v
		}
	}

	// Optional: HTTP retry wait max
	if retryWaitMax := os.Getenv("DO_HTTP_RETRY_WAIT_MAX"); retryWaitMax != "" {
		if v, err := time.ParseDuration(retryWaitMax); err == nil && v > 0 {
			cfg.HTTPRetryWaitMax = v
		}
	}

	// Optional: Workers
	if workers := os.Getenv("DO_WORKERS"); workers != "" {
		if v, err := strconv.Atoi(workers); err == nil && v > 0 {
			cfg.Workers = v
		}
	}

	return cfg, nil
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range split(s, sep) {
		if trimmed := trim(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
