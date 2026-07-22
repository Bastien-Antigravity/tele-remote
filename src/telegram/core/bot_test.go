package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------
// Mocks
// -----------------------------------------------------------------------------

type mockTelegramServer struct {
	server      *httptest.Server
	receivedMsg string
	mu          sync.Mutex
}

func newMockTelegramServer() *mockTelegramServer {
	ms := &mockTelegramServer{}
	ms.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ms.mu.Lock()
		defer ms.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
		} else if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			text := r.URL.Query().Get("text")
			if text == "" && r.Body != nil {
				var body struct {
					Text string `json:"text"`
				}
				if data, err := io.ReadAll(r.Body); err == nil {
					_ = json.Unmarshal(data, &body)
					text = body.Text
				}
			}
			ms.receivedMsg = text
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		} else {
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	return ms
}

func (ms *mockTelegramServer) Close() {
	ms.server.Close()
}

type MockLogger struct {
	unilog_ifaces.Logger
}

func (m *MockLogger) Debug(f string, a ...any)    {}
func (m *MockLogger) Info(f string, a ...any)     {}
func (m *MockLogger) Warning(f string, a ...any)  {}
func (m *MockLogger) Error(f string, a ...any)    {}
func (m *MockLogger) Critical(f string, a ...any) {}

// -----------------------------------------------------------------------------

type MockPublisher struct {
	LastCmdType int32
	LastPayload string
	Calls       int
}

func (m *MockPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	m.LastCmdType = cmdType
	m.LastPayload = payload
	m.Calls++
	return nil
}

func (m *MockPublisher) RequestRefresh(ctx context.Context) error {
	return nil
}

func (m *MockPublisher) Close() error { return nil }

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestBot_Broadcast(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	bot.Broadcast("Hello Telemetry")

	time.Sleep(50 * time.Millisecond)

	ms.mu.Lock()
	receivedMessage := ms.receivedMsg
	ms.mu.Unlock()

	if receivedMessage != "Hello Telemetry" {
		t.Errorf("Expected 'Hello Telemetry', got '%s'", receivedMessage)
	}
}
