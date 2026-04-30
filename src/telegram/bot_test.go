package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/config"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------
// Mocks
// -----------------------------------------------------------------------------

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

func (m *MockPublisher) PublishCommand(ctx context.Context, cmdType int32, payload string) error {
	m.LastCmdType = cmdType
	m.LastPayload = payload
	m.Calls++
	return nil
}

func (m *MockPublisher) Close() error { return nil }

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestBot_MenuRegistration(t *testing.T) {
	// 1. Setup Mock Telegram Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
	}))
	defer server.Close()

	// 2. Setup Config
	cfg, err := config.LoadConfig("test")
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}
	cfg.TelegramURL = server.URL
	cfg.ChatID = "12345678"
	
	logger := &MockLogger{}

	// 3. Init Bot
	bot, err := NewBot(cfg, logger, nil)
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	// 4. Test Menu Registration
	clientID := "test-client-1"
	menuJSON := `[
		{"label": "🚀 Start", "cmd_type": 1, "payload": "start_all"},
		{"label": "🛑 Stop", "cmd_type": 2}
	]`
	pub := &MockPublisher{}

	bot.OnComponentConnected(clientID, "TestService", menuJSON, pub)

	bot.mu.RLock()
	menu, ok := bot.dynamicMenus[clientID]
	bot.mu.RUnlock()

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

// -----------------------------------------------------------------------------

func TestBot_Broadcast(t *testing.T) {
	var receivedMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bot12345:TEST_TOKEN/sendMessage" {
			receivedMessage = r.URL.Query().Get("text")
			if receivedMessage == "" {
				var body struct {
					Text string `json:"text"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				receivedMessage = body.Text
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	cfg, _ := config.LoadConfig("test")
	cfg.TelegramURL = server.URL
	cfg.TelegramToken = "12345:TEST_TOKEN"
	cfg.ChatID = "12345678"

	bot, _ := NewBot(cfg, &MockLogger{}, nil)

	bot.Broadcast("Hello Telemetry")

	time.Sleep(200 * time.Millisecond)

	if receivedMessage != "Hello Telemetry" {
		t.Errorf("Expected 'Hello Telemetry', got '%s'", receivedMessage)
	}
}
