package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOptionsDefaults(t *testing.T) {
	o, err := parseOptions(nil)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", o.host)
	assert.Equal(t, 8080, o.port)
	assert.Equal(t, "0.0.0.0", o.healthHost)
	assert.Equal(t, 8888, o.healthPort)
}

func TestParseOptionsHealthFlagsWiredToServerConfig(t *testing.T) {
	o, err := parseOptions([]string{
		"--port=8080",
		"--host=localhost",
		"--health-port=8888",
		"--health-host=0.0.0.0",
		"--log-level=info",
	})
	require.NoError(t, err)

	cfg := o.serverConfig()

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "0.0.0.0", cfg.HealthHost)
	assert.Equal(t, 8888, cfg.HealthPort)
	assert.NotEqual(t, cfg.Port, cfg.HealthPort, "health server must listen on a separate port from the webhook API")
}
