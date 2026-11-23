package subscribers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/publishers"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_network "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/network"
	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// -----------------------------------------------------------------------------

type mockLogger struct {
	unilog_interfaces.Logger
}

func (m *mockLogger) Debug(f string, a ...any)    {}
func (m *mockLogger) Info(f string, a ...any)     {}
func (m *mockLogger) Warning(f string, a ...any)  {}
func (m *mockLogger) Error(f string, a ...any)    {}
func (m *mockLogger) Critical(f string, a ...any) {}

// -----------------------------------------------------------------------------

type mockConnectStream struct {
	grpc.ServerStream
	ctx     context.Context
	recvSeq []*grpc_control.ComponentMessage
	recvIdx int
	sendFn  func(*grpc_control.BotCommand) error
	mu      sync.Mutex
}

func (m *mockConnectStream) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *mockConnectStream) Recv() (*grpc_control.ComponentMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvIdx >= len(m.recvSeq) {
		return nil, io.EOF
	}
	msg := m.recvSeq[m.recvIdx]
	m.recvIdx++
	return msg, nil
}

func (m *mockConnectStream) Send(cmd *grpc_control.BotCommand) error {
	if m.sendFn != nil {
		return m.sendFn(cmd)
	}
	return nil
}

// -----------------------------------------------------------------------------

func TestConnect_RejectsTelemetryBeforeRegistration(t *testing.T) {
	sub := NewGrpcSubscriber(&mockLogger{}, "127.0.0.1", 0).(*GrpcSubscriber)
	stream := &mockConnectStream{
		ctx: context.Background(),
		recvSeq: []*grpc_control.ComponentMessage{
			{
				ComponentName: "svc-a",
				Host:          "127.0.0.1",
				Port:          9000,
				Payload: &grpc_control.ComponentMessage_Telemetry{
					Telemetry: "unregistered telemetry",
				},
			},
		},
	}

	err := sub.Connect(stream)
	if err == nil {
		t.Fatal("expected PermissionDenied error, got nil")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected codes.PermissionDenied, got %v (%v)", status.Code(err), err)
	}
	if !strings.Contains(err.Error(), "⛔ Unauthorized access") {
		t.Fatalf("expected error message to contain '⛔ Unauthorized access', got %q", err.Error())
	}
}

// -----------------------------------------------------------------------------

func TestConnect_AuthTokenEnforcement(t *testing.T) {
	sub := NewGrpcSubscriber(&mockLogger{}, "127.0.0.1", 0).(*GrpcSubscriber)
	if err := sub.ConfigureSecurity(toolbox_network.TLSPolicy{}, "secret-token-123"); err != nil {
		t.Fatalf("unexpected ConfigureSecurity error: %v", err)
	}

	regMsg := &grpc_control.ComponentMessage{
		ComponentName: "svc-auth",
		Host:          "127.0.0.1",
		Port:          8080,
		Payload: &grpc_control.ComponentMessage_Registration{
			Registration: &grpc_control.Registration{MenuJson: "{}"},
		},
	}

	// Missing token must be rejected
	noAuthStream := &mockConnectStream{
		ctx:     context.Background(),
		recvSeq: []*grpc_control.ComponentMessage{regMsg},
	}
	err := sub.Connect(noAuthStream)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied without token, got %v (%v)", status.Code(err), err)
	}
	if !strings.Contains(err.Error(), "⛔ Unauthorized access") {
		t.Fatalf("expected error to contain '⛔ Unauthorized access', got %q", err.Error())
	}

	// Invalid token must be rejected
	badAuthStream := &mockConnectStream{
		ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-tele-auth", "wrong-token")),
		recvSeq: []*grpc_control.ComponentMessage{regMsg},
	}
	err = sub.Connect(badAuthStream)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied with wrong token, got %v (%v)", status.Code(err), err)
	}

	// Valid token + Registration must succeed
	registered := false
	sub.cbs = tele_interfaces.ISubscriberCallbacks{
		OnRegistration: func(clientID, componentName, menuJSON string, pub tele_interfaces.IPublisher) {
			registered = true
		},
	}
	validAuthStream := &mockConnectStream{
		ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-tele-auth", "secret-token-123")),
		recvSeq: []*grpc_control.ComponentMessage{regMsg},
	}
	err = sub.Connect(validAuthStream)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected clean io.EOF after valid registration stream ends, got %v", err)
	}
	if !registered {
		t.Fatal("expected OnRegistration callback to be invoked for authorized client")
	}
}

// -----------------------------------------------------------------------------

func TestConnect_RejectsComponentIdentityMismatch(t *testing.T) {
	sub := NewGrpcSubscriber(&mockLogger{}, "127.0.0.1", 0).(*GrpcSubscriber)
	sub.cbs = tele_interfaces.ISubscriberCallbacks{
		OnRegistration: func(clientID, componentName, menuJSON string, pub tele_interfaces.IPublisher) {},
		OnTelemetry:    func(msg string) {},
	}

	stream := &mockConnectStream{
		ctx: context.Background(),
		recvSeq: []*grpc_control.ComponentMessage{
			{
				ComponentName: "service-a",
				Host:          "127.0.0.1",
				Port:          8081,
				Payload: &grpc_control.ComponentMessage_Registration{
					Registration: &grpc_control.Registration{MenuJson: "{}"},
				},
			},
			{
				ComponentName: "service-b",
				Host:          "127.0.0.1",
				Port:          8081,
				Payload: &grpc_control.ComponentMessage_Telemetry{
					Telemetry: "spoofed telemetry",
				},
			},
		},
	}

	err := sub.Connect(stream)
	if err == nil {
		t.Fatal("expected PermissionDenied on component identity mismatch, got nil")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected codes.PermissionDenied, got %v (%v)", status.Code(err), err)
	}
}

// -----------------------------------------------------------------------------

func TestGrpcPublisher_PublishCommandUnblocksOnContextTimeout(t *testing.T) {
	blockCh := make(chan struct{})
	t.Cleanup(func() {
		close(blockCh)
	})

	stream := &mockConnectStream{
		ctx: context.Background(),
		sendFn: func(cmd *grpc_control.BotCommand) error {
			<-blockCh
			return nil
		},
	}

	pub := publishers.NewGrpcPublisher(stream)
	shortCtx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := pub.PublishCommand(shortCtx, 1, "test-payload", "")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("PublishCommand blocked too long (%v) on stalled stream Send", elapsed)
	}
}

// -----------------------------------------------------------------------------

type peerOverrideConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (c *peerOverrideConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// -----------------------------------------------------------------------------

type testFleetPKI struct {
	CAFile         string
	ServerCertFile string
	ServerKeyFile  string
	ClientCertFile string
	ClientKeyFile  string
}

func generateTestFleetPKI(t *testing.T) testFleetPKI {
	t.Helper()
	dir := t.TempDir()

	caCert, caKey, caFile := writeTempCA(t, dir)
	srvCertFile, srvKeyFile := writeSignedCert(t, dir, "tele-remote", caCert, caKey, "tele-remote",
		[]string{"localhost", "tele-remote"},
		[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.1.50")},
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	)
	cliCertFile, cliKeyFile := writeSignedCert(t, dir, "client", caCert, caKey, "fleet-client",
		[]string{"fleet-client"},
		nil,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	)

	return testFleetPKI{
		CAFile:         caFile,
		ServerCertFile: srvCertFile,
		ServerKeyFile:  srvKeyFile,
		ClientCertFile: cliCertFile,
		ClientKeyFile:  cliKeyFile,
	}
}

// -----------------------------------------------------------------------------

func writeTempCA(t *testing.T, dir string) (*x509.Certificate, *ecdsa.PrivateKey, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("failed to generate CA serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Fleet Test Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	caFile := filepath.Join(dir, "ca.crt")
	caKeyFile := filepath.Join(dir, "ca.key")
	writeCertKeyPEM(t, caFile, caKeyFile, der, priv)
	return cert, priv, caFile
}

// -----------------------------------------------------------------------------

func writeSignedCert(
	t *testing.T,
	dir, prefix string,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	cn string,
	dnsNames []string,
	ips []net.IP,
	extUsage []x509.ExtKeyUsage,
) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("failed to generate leaf serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           extUsage,
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &priv.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to sign leaf cert %s: %v", prefix, err)
	}

	certFile := filepath.Join(dir, prefix+".crt")
	keyFile := filepath.Join(dir, prefix+".key")
	writeCertKeyPEM(t, certFile, keyFile, der, priv)
	return certFile, keyFile
}

// -----------------------------------------------------------------------------

func writeCertKeyPEM(t *testing.T, certPath, keyPath string, der []byte, priv *ecdsa.PrivateKey) {
	t.Helper()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("failed to write cert %s: %v", certPath, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal EC key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("failed to write key %s: %v", keyPath, err)
	}
}

// -----------------------------------------------------------------------------

func runCleartextHandshake(t *testing.T, creds credentials.TransportCredentials, remoteIP string) error {
	t.Helper()
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	_ = serverRaw.SetDeadline(time.Now().Add(2 * time.Second))
	_ = clientRaw.SetDeadline(time.Now().Add(2 * time.Second))

	go func() {
		_, _ = clientRaw.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	}()

	wrapped := &peerOverrideConn{
		Conn:       serverRaw,
		remoteAddr: &net.TCPAddr{IP: net.ParseIP(remoteIP), Port: 50000},
	}
	conn, _, err := creds.ServerHandshake(wrapped)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

// -----------------------------------------------------------------------------

func runTLSHandshake(t *testing.T, creds credentials.TransportCredentials, remoteIP string, clientTLS *tls.Config) error {
	t.Helper()
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	_ = serverRaw.SetDeadline(time.Now().Add(2 * time.Second))
	_ = clientRaw.SetDeadline(time.Now().Add(2 * time.Second))

	clientErrCh := make(chan error, 1)
	go func() {
		tlsCli := tls.Client(clientRaw, clientTLS)
		clientErrCh <- tlsCli.Handshake()
		_ = tlsCli.Close()
	}()

	wrapped := &peerOverrideConn{
		Conn:       serverRaw,
		remoteAddr: &net.TCPAddr{IP: net.ParseIP(remoteIP), Port: 50000},
	}
	conn, authInfo, serverErr := creds.ServerHandshake(wrapped)
	if conn != nil {
		_ = conn.Close()
	}
	clientErr := <-clientErrCh
	if serverErr != nil || clientErr != nil {
		return fmt.Errorf("serverErr=%v, clientErr=%v", serverErr, clientErr)
	}
	if _, ok := authInfo.(credentials.TLSInfo); !ok {
		return errors.New("expected credentials.TLSInfo on TLS handshake")
	}
	return nil
}

// -----------------------------------------------------------------------------

func TestGrpcSubscriber_ConfigureSecurity_FailFastAndCleartextBoot(t *testing.T) {
	sub := NewGrpcSubscriber(&mockLogger{}, "127.0.0.1", 0).(*GrpcSubscriber)

	// 1. Enabled: false with empty LocalTLS succeeds cleanly
	if err := sub.ConfigureSecurity(toolbox_network.TLSPolicy{
		Enabled: false,
		Mode:    "auto",
	}, ""); err != nil {
		t.Fatalf("expected cleartext ConfigureSecurity to succeed, got: %v", err)
	}
	if sub.dynCreds == nil {
		t.Fatal("expected dynCreds to be initialized on cleartext boot")
	}

	// 2. Enabled: true with missing cert/key fails fast
	err := sub.ConfigureSecurity(toolbox_network.TLSPolicy{
		Enabled: true,
		Mode:    "strict",
		LocalTLS: toolbox_config.LocalTLSConfig{
			CertFile: "/nonexistent/tele-remote.crt",
			KeyFile:  "/nonexistent/tele-remote.key",
		},
	}, "")
	if err == nil {
		t.Fatal("expected ConfigureSecurity to fail fast when TLS enabled with missing cert/key")
	}
}

// -----------------------------------------------------------------------------

func TestGrpcSubscriber_OptionAMTLSAndRuntimeUpdate(t *testing.T) {
	pki := generateTestFleetPKI(t)
	localTLS := toolbox_config.LocalTLSConfig{
		CertFile: pki.ServerCertFile,
		KeyFile:  pki.ServerKeyFile,
		CAFile:   pki.CAFile,
	}

	sub := NewGrpcSubscriber(&mockLogger{}, "127.0.0.1", 0).(*GrpcSubscriber)
	if err := sub.ConfigureSecurity(toolbox_network.TLSPolicy{
		Enabled:  true,
		Mode:     "auto",
		LocalTLS: localTLS,
	}, ""); err != nil {
		t.Fatalf("expected Option A ConfigureSecurity to succeed, got: %v", err)
	}

	clientTLS, err := toolbox_network.BuildClientTLSConfig(toolbox_config.LocalTLSConfig{
		CertFile: pki.ClientCertFile,
		KeyFile:  pki.ClientKeyFile,
		CAFile:   pki.CAFile,
	}, "192.168.1.50")
	if err != nil {
		t.Fatalf("failed to build client TLS config: %v", err)
	}

	// 1. In "auto" mode: local cleartext succeeds, remote cleartext is rejected ("⛔ Unauthorized access")
	if err := runCleartextHandshake(t, sub.dynCreds, "127.0.0.1"); err != nil {
		t.Fatalf("expected local cleartext to succeed in auto mode, got: %v", err)
	}
	if err := runCleartextHandshake(t, sub.dynCreds, "192.168.1.50"); err == nil {
		t.Fatal("expected remote cleartext to be rejected in auto mode")
	} else if !strings.Contains(err.Error(), "⛔ Unauthorized access") {
		t.Fatalf("expected remote rejection to contain '⛔ Unauthorized access', got: %v", err)
	}

	// 2. Remote client with CA-signed client cert succeeds mTLS handshake
	if err := runTLSHandshake(t, sub.dynCreds, "192.168.1.50", clientTLS); err != nil {
		t.Fatalf("expected remote mTLS handshake with CA-signed client cert to succeed, got: %v", err)
	}

	// 3. UpdateRuntimeSecurity switches to "strict" (rejecting local cleartext) and preserves active TLS on invalid reload
	if err := sub.UpdateRuntimeSecurity(toolbox_network.TLSPolicy{
		Enabled:  true,
		Mode:     "strict",
		LocalTLS: localTLS,
	}, "rotated-token"); err != nil {
		t.Fatalf("expected UpdateRuntimeSecurity to strict to succeed, got: %v", err)
	}
	if err := runCleartextHandshake(t, sub.dynCreds, "127.0.0.1"); err == nil {
		t.Fatal("expected local cleartext to be rejected in strict mode")
	} else if !strings.Contains(err.Error(), "⛔ Unauthorized access: strict TLS required") {
		t.Fatalf("unexpected strict rejection error: %v", err)
	}

	// Invalid cert path must fail and preserve previous strict TLS policy
	err = sub.UpdateRuntimeSecurity(toolbox_network.TLSPolicy{
		Enabled: true,
		Mode:    "auto",
		LocalTLS: toolbox_config.LocalTLSConfig{
			CertFile: "/nonexistent/bad.crt",
			KeyFile:  "/nonexistent/bad.key",
		},
	}, "ignored-token")
	if err == nil {
		t.Fatal("expected UpdateRuntimeSecurity with invalid cert path to fail")
	}

	if err := runCleartextHandshake(t, sub.dynCreds, "127.0.0.1"); err == nil {
		t.Fatal("expected preserved strict TLS config to still reject local cleartext")
	} else if !strings.Contains(err.Error(), "⛔ Unauthorized access: strict TLS required") {
		t.Fatalf("unexpected rejection error after failed reload: %v", err)
	}
	if err := runTLSHandshake(t, sub.dynCreds, "192.168.1.50", clientTLS); err != nil {
		t.Fatalf("expected preserved strict mTLS handshake to succeed after failed reload, got: %v", err)
	}
}
