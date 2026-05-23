// Package testwt boots a real room WebTransport server on a loopback port, so tests can exercise the room data plane (handshake, datagrams, streams) end to end rather than mocking the transport.
package testwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
	roomhandler "github.com/averak/vfx/internal/presentation/room"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

const jwtSecret = "testwt-secret"

// Room is a running room server addressable at Endpoint, with the Signer that mints session tokens it will accept.
type Room struct {
	Endpoint string
	Signer   *token.Signer
}

// New starts a room.Server backed by factory on a free loopback UDP port with a fresh self-signed certificate.
// It is torn down via t.Cleanup; the returned Endpoint may need a short connect retry while the listener binds.
func New(t *testing.T, factory plugin.Factory) *Room {
	t.Helper()

	certFile, keyFile := writeSelfSignedCert(t)
	addr := net.JoinHostPort("127.0.0.1", freeUDPPort(t))
	cfg := &config.Room{
		ListenAddr:       addr,
		TLSCertFile:      certFile,
		TLSKeyFile:       keyFile,
		JWTSecret:        jwtSecret,
		HandshakeTimeout: 5 * time.Second,
		DatagramMaxBytes: 1200,
	}
	signer := token.NewSigner(jwtSecret)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	manager := usecaseroom.NewManager(ctx, factory, logger, nil)
	srv, err := roomhandler.NewServer(cfg, signer, manager, logger)
	if err != nil {
		cancel()
		t.Fatalf("testwt: new server: %v", err)
	}
	go func() { _ = srv.ListenAndServe(ctx) }() //nolint:errcheck // stops when ctx is cancelled in cleanup.
	t.Cleanup(func() {
		cancel()
		manager.Close()
	})

	return &Room{Endpoint: addr, Signer: signer}
}

func freeUDPPort(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("testwt: reserve udp port: %v", err)
	}
	defer func() { _ = conn.Close() }() //nolint:errcheck // probe socket, only used to pick a free port.
	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("testwt: split port: %v", err)
	}
	return port
}

func writeSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testwt: generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vfx-testwt"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("testwt: create cert: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("testwt: marshal key: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "PRIVATE KEY", keyDER)
	return certFile, keyFile
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("testwt: write %s: %v", path, err)
	}
}
