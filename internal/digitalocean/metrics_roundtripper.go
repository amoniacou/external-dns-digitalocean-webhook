package digitalocean

import (
	"net/http"

	"github.com/amoniacou/external-dns-digitalocean-webhook/internal/metrics"
	log "github.com/sirupsen/logrus"
)

type metricsRoundTripper struct {
	base http.RoundTripper
}

func (m *metricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := m.base.RoundTrip(req)
	
	metrics.DigitalOceanRequestsTotal.Inc()
	
	if err != nil {
		metrics.DigitalOceanErrorsTotal.Inc()
		log.WithFields(log.Fields{
			"method": req.Method,
			"url":    req.URL.String(),
		}).WithError(err).Error("DigitalOcean API request failed")
		return resp, err
	}
	
	if resp.StatusCode == http.StatusTooManyRequests {
		metrics.DigitalOceanRateLimitsTotal.Inc()
		log.WithFields(log.Fields{
			"method":     req.Method,
			"url":        req.URL.String(),
			"status":     resp.StatusCode,
			"request_id": resp.Header.Get("X-Request-Id"),
			"reset":      resp.Header.Get("RateLimit-Reset"),
			"remaining":  resp.Header.Get("RateLimit-Remaining"),
		}).Warn("DigitalOcean rate limit hit")
	}
	
	if resp.StatusCode >= 400 {
		metrics.DigitalOceanErrorsTotal.Inc()
		
		// Don't double log 429 as error if we already logged it as warn, or keep it for consistency
		if resp.StatusCode != http.StatusTooManyRequests {
			log.WithFields(log.Fields{
				"method":     req.Method,
				"url":        req.URL.String(),
				"status":     resp.StatusCode,
				"request_id": resp.Header.Get("X-Request-Id"),
			}).Error("DigitalOcean API returned error")
		}
	}
	
	return resp, err
}