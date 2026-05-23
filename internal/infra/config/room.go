package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Room holds every value the room daemon needs to start.
type Room struct {
	// ListenAddr is the UDP address the WebTransport server binds to.
	ListenAddr string `env:"VFX_ROOM_LISTEN_ADDR" envDefault:":7777"`

	// TLSCertFile / TLSKeyFile point at a PEM certificate and key.
	// Required because WebTransport is HTTP/3 over QUIC, which mandates
	// TLS. For local development, mkcert or a self-signed pair both
	// work; production deployments inject managed certificates here.
	TLSCertFile string `env:"VFX_ROOM_TLS_CERT,required,notEmpty"`
	TLSKeyFile  string `env:"VFX_ROOM_TLS_KEY,required,notEmpty"`

	// JWTSecret must match the gateway's VFX_JWT_SECRET so session
	// tokens minted there verify here.
	JWTSecret string `env:"VFX_JWT_SECRET,required,notEmpty"`

	// TickRateHz is the fallback tick rate used when a plugin's
	// requested rate is zero. Setting it here lets operators bound a
	// runaway plugin.
	TickRateHz int `env:"VFX_ROOM_TICK_RATE_HZ" envDefault:"30"`

	// PluginPath is the filesystem path to the WASM plugin loaded at
	// startup. A daemon hosts a single plugin per process; running
	// multiple game modes uses one Fleet (and image) per mode.
	PluginPath string `env:"VFX_ROOM_PLUGIN_PATH"`

	// HandshakeTimeout caps how long the daemon will wait for the
	// WebTransport handshake to complete before tearing the connection
	// down.
	HandshakeTimeout time.Duration `env:"VFX_ROOM_HANDSHAKE_TIMEOUT" envDefault:"10s"`

	// AgonesEnabled turns on the Agones game-server SDK: the daemon marks
	// itself Ready, sends health pings, and Shutdown on exit. Leave it
	// off for compose/local runs where no Agones SDK sidecar exists.
	AgonesEnabled bool `env:"VFX_ROOM_AGONES_ENABLED" envDefault:"false"`

	// AgonesHealthInterval is how often the daemon pings the Agones SDK
	// health stream. Must be shorter than the Fleet's health.periodSeconds.
	AgonesHealthInterval time.Duration `env:"VFX_ROOM_AGONES_HEALTH_INTERVAL" envDefault:"2s"`

	// MetricsAddr is the TCP address for the room's HTTP/1.1 metrics and
	// probe server (/metrics, /healthz, /readyz). The WebTransport tier
	// is HTTP/3 over UDP, so Prometheus scraping needs this separate
	// plain-HTTP listener.
	MetricsAddr string `env:"VFX_ROOM_METRICS_ADDR" envDefault:":9090"`

	// DatagramMaxBytes is the largest marshalled frame sent as an
	// unreliable datagram; larger frames (e.g. full snapshots) go over a
	// reliable WebTransport stream instead. Defaults to a conservative
	// size that fits a typical QUIC datagram. Lower it to exercise the
	// stream path in tests.
	DatagramMaxBytes int `env:"VFX_ROOM_DATAGRAM_MAX_BYTES" envDefault:"1200"`
}

// LoadRoom reads the room configuration from the environment.
func LoadRoom() (*Room, error) {
	var cfg Room
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}
