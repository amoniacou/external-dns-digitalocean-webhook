package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DigitalOceanRequestsTotal counts the total number of requests to DigitalOcean API
	DigitalOceanRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "digitalocean_api_requests_total",
		Help: "Total number of requests to DigitalOcean API",
	})

	// DigitalOceanErrorsTotal counts the total number of failed requests to DigitalOcean API
	DigitalOceanErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "digitalocean_api_errors_total",
		Help: "Total number of failed requests to DigitalOcean API",
	})

	// DigitalOceanRateLimitsTotal counts the total number of times we hit rate limits (429)
	DigitalOceanRateLimitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "digitalocean_api_rate_limits_total",
		Help: "Total number of rate limit hits (429) from DigitalOcean API",
	})
)