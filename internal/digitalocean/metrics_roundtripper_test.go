package digitalocean

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/amoniacou/external-dns-digitalocean-webhook/internal/metrics"
)

type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func TestMetricsRoundTripper(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		respStatusCode int
		respErr        error
		wantReqDelta   int
		wantErrDelta   int
		wantRateDelta  int
	}{
		{
			name:           "success request",
			method:         "GET",
			path:           "/v2/domains",
			respStatusCode: http.StatusOK,
			wantReqDelta:   1,
			wantErrDelta:   0,
			wantRateDelta:  0,
		},
		{
			name:           "error request (network)",
			method:         "POST",
			path:           "/v2/domains",
			respErr:        errors.New("network error"),
			wantReqDelta:   1,
			wantErrDelta:   1,
			wantRateDelta:  0,
		},
		{
			name:           "rate limit request",
			method:         "GET",
			path:           "/v2/domains",
			respStatusCode: http.StatusTooManyRequests,
			wantReqDelta:   1,
			wantErrDelta:   1, 
			wantRateDelta:  1,
		},
		{
			name:           "server error request",
			method:         "DELETE",
			path:           "/v2/domains/example.com",
			respStatusCode: http.StatusInternalServerError,
			wantReqDelta:   1,
			wantErrDelta:   1,
			wantRateDelta:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get initial values
			initReq := testutil.ToFloat64(metrics.DigitalOceanRequestsTotal)
			initErr := testutil.ToFloat64(metrics.DigitalOceanErrorsTotal)
			initRate := testutil.ToFloat64(metrics.DigitalOceanRateLimitsTotal)

			req := httptest.NewRequest(tt.method, "http://api.digitalocean.com"+tt.path, nil)
			resp := &http.Response{StatusCode: tt.respStatusCode}
			mockRT := &mockRoundTripper{resp: resp, err: tt.respErr}
			metricsRT := &metricsRoundTripper{base: mockRT}
			
			_, _ = metricsRT.RoundTrip(req)
			
			assert.Equal(t, initReq+float64(tt.wantReqDelta), testutil.ToFloat64(metrics.DigitalOceanRequestsTotal))
			assert.Equal(t, initErr+float64(tt.wantErrDelta), testutil.ToFloat64(metrics.DigitalOceanErrorsTotal))
			assert.Equal(t, initRate+float64(tt.wantRateDelta), testutil.ToFloat64(metrics.DigitalOceanRateLimitsTotal))
		})
	}
}

func TestMetricsRoundTripper_Integration(t *testing.T) {
    initReq := testutil.ToFloat64(metrics.DigitalOceanRequestsTotal)
    initErr := testutil.ToFloat64(metrics.DigitalOceanErrorsTotal)
    
    req := httptest.NewRequest("GET", "http://example.com/foo", nil)
    resp := &http.Response{StatusCode: 200}
    rt := &metricsRoundTripper{base: &mockRoundTripper{resp: resp}}
    
    _, err := rt.RoundTrip(req)
    require.NoError(t, err)
    
    assert.Equal(t, initReq+1, testutil.ToFloat64(metrics.DigitalOceanRequestsTotal))
    assert.Equal(t, initErr, testutil.ToFloat64(metrics.DigitalOceanErrorsTotal))
}