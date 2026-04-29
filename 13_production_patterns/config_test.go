package productionpatterns

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Defaults(t *testing.T) {
	//👉 Without env variables - defaults are used
	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 15*time.Second, cfg.ReadTimeout)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 25, cfg.DatabaseMaxConns)
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	//👉 Env variables overwrite defaults
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_JSON", "true")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.LogJSON)
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv("HTTP_PORT", "not_a_number")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP_PORT")
}

func TestLoadConfig_PortOutOfRange(t *testing.T) {
	t.Setenv("HTTP_PORT", "99999")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1-65535")
}

func TestLoadConfig_InvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_LEVEL")
}

func TestAppConfig_Addr(t *testing.T) {
	cfg := &AppConfig{HTTPPort: 3000}
	assert.Equal(t, ":3000", cfg.Addr())
}

// =============================================================================
//Functional Options tests
// =============================================================================

func TestNewServerConfig_Defaults(t *testing.T) {
	cfg := NewServerConfig()

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 15*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 100, cfg.MaxConns)
}

func TestNewServerConfig_WithOptions(t *testing.T) {
	cfg := NewServerConfig(
		WithPort(9090),
		WithReadTimeout(30*time.Second),
		WithMaxConns(200),
	)

	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 200, cfg.MaxConns)
	//👉 WriteTimeout remains default
	assert.Equal(t, 15*time.Second, cfg.WriteTimeout)
}
