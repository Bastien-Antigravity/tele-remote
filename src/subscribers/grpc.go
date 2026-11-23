package subscribers

/*
ESSENTIAL PROCESS:
Implements the gRPC transport listener for Tele-Remote, delegating dynamic
local/remote TLS/mTLS policy enforcement to microservice-toolbox while enforcing
bearer token authentication, registration-first stream gating, and bounded shutdown.

DATA FLOW:
1. ConfigureSecurity initializes DynamicTransportCredentials synchronously on the cold path.
2. StartListen configures WireTap interceptors and DynamicTransportCredentials, binds TCP, and serves TeleRemoteServiceServer.
3. UpdateRuntimeSecurity hot-reloads authToken and TLS policy via DynamicTransportCredentials.UpdatePolicy.
4. Connect enforces optional authToken validation, requires a valid Registration frame before telemetry/qmsg frames, and pins component identity.
5. Shutdown closes stopCh to unblock active streams and runs GracefulStop bounded by gracefulTimeout before forcing Stop.

KEY PARAMETERS:
- bindIP/bindPort: Configured IP/hostname and TCP port for the gRPC server listener.
- cbs: Callback hooks linking transport events to the Telegram bot core.
- tlsPolicy/dynCreds: Runtime-updatable Option A X.509 TLS/mTLS policy and credentials.
- authToken: Optional shared secret validated against x-tele-auth or authorization metadata.
*/

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/publishers"

	toolbox_network "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/network"
	toolbox_wiretap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/wiretap"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// -----------------------------------------------------------------------------

type streamRecvResult struct {
	msg *grpc_control.ComponentMessage
	err error
}

// -----------------------------------------------------------------------------

// GrpcSubscriber acts as the Tele-Remote generic gRPC Listener
type GrpcSubscriber struct {
	grpc_control.UnimplementedTeleRemoteServiceServer
	log             interfaces.Logger
	bindIP          string
	bindPort        int
	cbs             tele_interfaces.ISubscriberCallbacks
	grpcSrv         *grpc.Server
	mu              sync.Mutex
	tlsPolicy       toolbox_network.TLSPolicy
	dynCreds        *toolbox_network.DynamicTransportCredentials
	authToken       string
	gracefulTimeout time.Duration
	stopCh          chan struct{}
	stopOnce        sync.Once
}

// -----------------------------------------------------------------------------

// NewGrpcSubscriber instantiates a gRPC transport listener bound to ip:port.
func NewGrpcSubscriber(l interfaces.Logger, ip string, port int) tele_interfaces.ISubscriber {
	return &GrpcSubscriber{
		log:             l,
		bindIP:          ip,
		bindPort:        port,
		tlsPolicy:       toolbox_network.TLSPolicy{Mode: "auto"},
		gracefulTimeout: 5 * time.Second,
		stopCh:          make(chan struct{}),
	}
}

// -----------------------------------------------------------------------------

// ConfigureSecurity validates and initializes Option A TLS/mTLS credentials and auth token on the cold path.
func (s *GrpcSubscriber) ConfigureSecurity(policy toolbox_network.TLSPolicy, authToken string) error {
	dynCreds, err := toolbox_network.NewDynamicTransportCredentials(s.bindIP, policy)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.tlsPolicy = policy
	s.dynCreds = dynCreds
	s.authToken = authToken
	s.mu.Unlock()
	return nil
}

// -----------------------------------------------------------------------------

// UpdateRuntimeSecurity updates TLS enforcement policy and auth token dynamically while retaining valid active creds on error.
func (s *GrpcSubscriber) UpdateRuntimeSecurity(policy toolbox_network.TLSPolicy, authToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dynCreds == nil {
		dynCreds, err := toolbox_network.NewDynamicTransportCredentials(s.bindIP, policy)
		if err != nil {
			return err
		}
		s.dynCreds = dynCreds
	} else if err := s.dynCreds.UpdatePolicy(policy); err != nil {
		return err
	}

	s.tlsPolicy = policy
	s.authToken = authToken
	return nil
}

// -----------------------------------------------------------------------------

// StartListen binds the TCP socket and serves incoming gRPC component streams.
func (s *GrpcSubscriber) StartListen(ctx context.Context, cbs tele_interfaces.ISubscriberCallbacks) error {
	s.mu.Lock()
	s.cbs = cbs
	creds := s.dynCreds
	policy := s.tlsPolicy
	bindIP := s.bindIP
	s.mu.Unlock()

	if creds == nil {
		var err error
		creds, err = toolbox_network.NewDynamicTransportCredentials(bindIP, policy)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.dynCreds = creds
		s.mu.Unlock()
	}

	opts := append([]grpc.ServerOption{grpc.Creds(creds)}, toolbox_wiretap.ServerOptions()...)

	addr := fmt.Sprintf("%s:%d", s.bindIP, s.bindPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Error("gRPC Subscriber failed to listen", "err", err, "addr", addr)
		return err
	}

	srv := grpc.NewServer(opts...)
	s.mu.Lock()
	s.grpcSrv = srv
	s.mu.Unlock()

	grpc_control.RegisterTeleRemoteServiceServer(srv, s)

	go func() {
		<-ctx.Done()
		s.log.Info("Shutting down gRPC Subscriber gently")
		s.shutdown()
	}()

	s.log.Info("Starting gRPC Subscriber", "addr", addr)
	return srv.Serve(lis)
}

// -----------------------------------------------------------------------------

func validateStreamAuth(ctx context.Context, expectedToken string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}

	expected := []byte(expectedToken)
	for _, val := range md.Get("x-tele-auth") {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(val)), expected) == 1 {
			return true
		}
	}

	for _, val := range md.Get("authorization") {
		trimmed := strings.TrimSpace(val)
		if len(trimmed) >= 7 && strings.EqualFold(trimmed[:7], "bearer ") {
			trimmed = strings.TrimSpace(trimmed[7:])
		}
		if subtle.ConstantTimeCompare([]byte(trimmed), expected) == 1 {
			return true
		}
	}

	return false
}

// -----------------------------------------------------------------------------

// Connect implements the grpc_control.TeleRemoteServiceServer stream loop
func (s *GrpcSubscriber) Connect(stream grpc_control.TeleRemoteService_ConnectServer) error {
	s.mu.Lock()
	authToken := s.authToken
	cbs := s.cbs
	s.mu.Unlock()

	if authToken != "" && !validateStreamAuth(stream.Context(), authToken) {
		s.log.Warning("Unauthorized gRPC client connection rejected: invalid auth token")
		return status.Errorf(codes.PermissionDenied, "⛔ Unauthorized access: invalid gRPC client token")
	}

	var clientID string
	registered := false
	var registeredComponent string

	// Wrap stream as publisher immediately
	dynPub := publishers.NewGrpcPublisher(stream)

	recvCh := make(chan streamRecvResult)
	doneCh := make(chan struct{})
	defer close(doneCh)

	go func() {
		for {
			msg, err := stream.Recv()
			select {
			case recvCh <- streamRecvResult{msg: msg, err: err}:
				if err != nil {
					return
				}
			case <-doneCh:
				return
			}
		}
	}()

	notifyDisconnect := func() {
		if clientID != "" && cbs.OnDisconnect != nil {
			go cbs.OnDisconnect(clientID, dynPub)
		}
	}

	for {
		var msg *grpc_control.ComponentMessage
		var err error

		select {
		case <-s.stopCh:
			notifyDisconnect()
			return status.Error(codes.Unavailable, "gRPC server shutting down")
		case <-stream.Context().Done():
			notifyDisconnect()
			return stream.Context().Err()
		case res := <-recvCh:
			msg, err = res.msg, res.err
		}

		if err != nil {
			s.log.Warning("stream.Recv error or client disconnected", "client", clientID, "err", err)
			notifyDisconnect()
			return err
		}

		if msg == nil {
			continue
		}

		compName := strings.TrimSpace(msg.ComponentName)
		if !registered {
			_, isReg := msg.Payload.(*grpc_control.ComponentMessage_Registration)
			if !isReg || compName == "" || len(compName) > 64 || msg.Port < 0 || msg.Port > 65535 {
				s.log.Warning("Unauthorized gRPC stream frame before registration", "component", compName)
				return status.Errorf(codes.PermissionDenied, "⛔ Unauthorized access: component registration required before sending telemetry")
			}
			registered = true
			registeredComponent = compName
			clientID = fmt.Sprintf("%s:%s:%d", compName, strings.TrimSpace(msg.Host), msg.Port)
			s.log.Info("New client connected via gRPC", "id", clientID)
		} else if compName != "" && compName != registeredComponent {
			s.log.Warning("Unauthorized gRPC stream frame: component identity mismatch", "expected", registeredComponent, "got", compName)
			notifyDisconnect()
			return status.Errorf(codes.PermissionDenied, "⛔ Unauthorized access: component identity mismatch on active stream")
		}

		switch payload := msg.Payload.(type) {
		case *grpc_control.ComponentMessage_Registration:
			if cbs.OnRegistration != nil {
				reg := payload.Registration
				menuJSON := ""
				if reg != nil {
					menuJSON = reg.MenuJson
				}
				cbs.OnRegistration(clientID, compName, menuJSON, dynPub)
			}
		case *grpc_control.ComponentMessage_Telemetry:
			if cbs.OnTelemetry != nil {
				cbs.OnTelemetry(payload.Telemetry)
			}
		case *grpc_control.ComponentMessage_Qmsg:
			if cbs.OnTelemetry != nil && payload.Qmsg != nil {
				q := payload.Qmsg
				cbs.OnTelemetry(fmt.Sprintf("%s => %s: %s", q.FromAddr, q.ToAddr, q.Msg))
			}
		}
	}
}

// -----------------------------------------------------------------------------

func (s *GrpcSubscriber) shutdown() {
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
		s.mu.Lock()
		srv := s.grpcSrv
		timeout := s.gracefulTimeout
		s.mu.Unlock()
		if srv == nil {
			return
		}
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(timeout):
			s.log.Warning("gRPC GracefulStop timed out; forcing immediate Stop", "timeout", timeout)
			srv.Stop()
		}
	})
}

// -----------------------------------------------------------------------------

// Close gracefully stops the gRPC server within the bounded graceful timeout.
func (s *GrpcSubscriber) Close() error {
	s.shutdown()
	return nil
}
