package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

type mockTelegramServer struct {
	server *httptest.Server
	mu     sync.Mutex
}

func newMockTelegramServer() *mockTelegramServer {
	ms := &mockTelegramServer{}
	ms.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
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

func TestBot_MenuRegistration(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	logger := &MockLogger{}
	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", logger)
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	clientID := "test-client-1"
	menuJSON := `[
		{"label": "🚀 Start", "cmd_type": 1, "payload": "start_all"},
		{"label": "🛑 Stop", "cmd_type": 2}
	]`
	pub := &MockPublisher{}

	OnComponentConnected(bot, clientID, "TestService", menuJSON, pub)

	bot.Mu.RLock()
	menu, ok := bot.DynamicMenus[clientID]
	bot.Mu.RUnlock()

	if !ok {
		t.Fatal("Menu was not registered")
	}

	if menu.Name != "TestService" {
		t.Errorf("Expected name TestService, got %s", menu.Name)
	}

	if len(menu.Root.Rows) != 2 {
		t.Errorf("Expected 2 menu rows, got %d", len(menu.Root.Rows))
	}
}
