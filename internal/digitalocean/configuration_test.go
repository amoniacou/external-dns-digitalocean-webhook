package digitalocean

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.APIPageSize != 200 {
		t.Errorf("expected APIPageSize=200, got %d", cfg.APIPageSize)
	}
	if cfg.HTTPRetryMax != 3 {
		t.Errorf("expected HTTPRetryMax=3, got %d", cfg.HTTPRetryMax)
	}
	if cfg.HTTPRetryWaitMin != 1*time.Second {
		t.Errorf("expected HTTPRetryWaitMin=1s, got %s", cfg.HTTPRetryWaitMin)
	}
	if cfg.HTTPRetryWaitMax != 30*time.Second {
		t.Errorf("expected HTTPRetryWaitMax=30s, got %s", cfg.HTTPRetryWaitMax)
	}
	if cfg.Workers != 10 {
		t.Errorf("expected Workers=10, got %d", cfg.Workers)
	}
}

func TestNewConfigFromEnv_RequiredToken(t *testing.T) {
	// Clear the environment
	os.Unsetenv("DO_TOKEN")

	_, err := NewConfigFromEnv()
	if err == nil {
		t.Error("expected error when DO_TOKEN is not set")
	}
}

func TestNewConfigFromEnv_WithToken(t *testing.T) {
	os.Setenv("DO_TOKEN", "test-token")
	defer os.Unsetenv("DO_TOKEN")

	cfg, err := NewConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIToken != "test-token" {
		t.Errorf("expected APIToken=test-token, got %s", cfg.APIToken)
	}
}

func TestNewConfigFromEnv_DomainFilter(t *testing.T) {
	os.Setenv("DO_TOKEN", "test-token")
	os.Setenv("DO_DOMAIN_FILTER", "example.com, example.org, test.io")
	defer func() {
		os.Unsetenv("DO_TOKEN")
		os.Unsetenv("DO_DOMAIN_FILTER")
	}()

	cfg, err := NewConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"example.com", "example.org", "test.io"}
	if len(cfg.DomainFilter) != len(expected) {
		t.Fatalf("expected %d domains, got %d", len(expected), len(cfg.DomainFilter))
	}
	for i, domain := range expected {
		if cfg.DomainFilter[i] != domain {
			t.Errorf("expected domain[%d]=%s, got %s", i, domain, cfg.DomainFilter[i])
		}
	}
}

func TestNewConfigFromEnv_DryRun(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("DO_TOKEN", "test-token")
			if tt.envValue != "" {
				os.Setenv("DO_DRY_RUN", tt.envValue)
			} else {
				os.Unsetenv("DO_DRY_RUN")
			}
			defer func() {
				os.Unsetenv("DO_TOKEN")
				os.Unsetenv("DO_DRY_RUN")
			}()

			cfg, err := NewConfigFromEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.DryRun != tt.expected {
				t.Errorf("expected DryRun=%v, got %v", tt.expected, cfg.DryRun)
			}
		})
	}
}

func TestNewConfigFromEnv_APIPageSize(t *testing.T) {
	os.Setenv("DO_TOKEN", "test-token")
	os.Setenv("DO_API_PAGE_SIZE", "100")
	defer func() {
		os.Unsetenv("DO_TOKEN")
		os.Unsetenv("DO_API_PAGE_SIZE")
	}()

	cfg, err := NewConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIPageSize != 100 {
		t.Errorf("expected APIPageSize=100, got %d", cfg.APIPageSize)
	}
}

func TestNewConfigFromEnv_HTTPRetrySettings(t *testing.T) {
	os.Setenv("DO_TOKEN", "test-token")
	os.Setenv("DO_HTTP_RETRY_MAX", "5")
	os.Setenv("DO_HTTP_RETRY_WAIT_MIN", "2s")
	os.Setenv("DO_HTTP_RETRY_WAIT_MAX", "60s")
	defer func() {
		os.Unsetenv("DO_TOKEN")
		os.Unsetenv("DO_HTTP_RETRY_MAX")
		os.Unsetenv("DO_HTTP_RETRY_WAIT_MIN")
		os.Unsetenv("DO_HTTP_RETRY_WAIT_MAX")
	}()

	cfg, err := NewConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.HTTPRetryMax != 5 {
		t.Errorf("expected HTTPRetryMax=5, got %d", cfg.HTTPRetryMax)
	}
	if cfg.HTTPRetryWaitMin != 2*time.Second {
		t.Errorf("expected HTTPRetryWaitMin=2s, got %s", cfg.HTTPRetryWaitMin)
	}
	if cfg.HTTPRetryWaitMax != 60*time.Second {
		t.Errorf("expected HTTPRetryWaitMax=60s, got %s", cfg.HTTPRetryWaitMax)
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{"a , b , c", ",", []string{"a", "b", "c"}},
		{"  a  ,  b  ,  c  ", ",", []string{"a", "b", "c"}},
		{"", ",", nil},
		{"single", ",", []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d items, got %d", len(tt.expected), len(result))
			}
			for i, v := range tt.expected {
				if result[i] != v {
					t.Errorf("expected result[%d]=%s, got %s", i, v, result[i])
				}
			}
		})
	}
}
