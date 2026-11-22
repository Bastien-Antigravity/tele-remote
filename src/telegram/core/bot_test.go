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

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------
// Mocks
// -----------------------------------------------------------------------------

type mockTelegramServer struct {
	server      *httptest.Server
	receivedMsg string
	messages    []string
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
			ms.messages = append(ms.messages, text)
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

func (ms *mockTelegramServer) lastMessage() string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.receivedMsg
}

type MockLogger struct {
	unilog_ifaces.Logger
	mu       sync.Mutex
	warnings []string
}

func (m *MockLogger) Debug(f string, a ...any)   {}
func (m *MockLogger) Info(f string, a ...any)    {}
func (m *MockLogger) Warning(f string, a ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warnings = append(m.warnings, f)
}
func (m *MockLogger) Error(f string, a ...any)    {}
func (m *MockLogger) Critical(f string, a ...any) {}

// -----------------------------------------------------------------------------

type MockPublisher struct {
	LastCmdType int32
	LastPayload string
	LastInput   string
	Calls       int
}

func (m *MockPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	m.LastCmdType = cmdType
	m.LastPayload = payload
	m.LastInput = input
	m.Calls++
	return nil
}

func (m *MockPublisher) RequestRefresh(ctx context.Context) error {
	return nil
}

func (m *MockPublisher) Close() error { return nil }

// -----------------------------------------------------------------------------

type fakeContext struct {
	tb.Context
	chatID   int64
	userID   int64
	text     string
	sentMsgs []string
}

func newFakeContext(chatID, userID int64, text string) *fakeContext {
	return &fakeContext{
		chatID: chatID,
		userID: userID,
		text:   text,
	}
}

func (f *fakeContext) Chat() *tb.Chat {
	return &tb.Chat{ID: f.chatID}
}

func (f *fakeContext) Sender() *tb.User {
	if f.userID == 0 {
		return nil
	}
	return &tb.User{ID: f.userID}
}

func (f *fakeContext) Message() *tb.Message {
	return &tb.Message{
		Text:   f.text,
		Chat:   f.Chat(),
		Sender: f.Sender(),
	}
}

func (f *fakeContext) Callback() *tb.Callback {
	return nil
}

func (f *fakeContext) Text() string {
	return f.text
}

func (f *fakeContext) Send(what interface{}, opts ...interface{}) error {
	if s, ok := what.(string); ok {
		f.sentMsgs = append(f.sentMsgs, s)
	}
	return nil
}

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

	if got := ms.lastMessage(); got != "Hello Telemetry" {
		t.Errorf("Expected 'Hello Telemetry', got '%s'", got)
	}
}

// -----------------------------------------------------------------------------

func TestBot_OnTelemetryNonBlocking(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	bot.OnTelemetry("Async Telemetry Message")

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if ms.lastMessage() == "Async Telemetry Message" {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("Expected async telemetry broadcast to deliver 'Async Telemetry Message', got '%s'", ms.lastMessage())
}

// -----------------------------------------------------------------------------

func TestBot_AuthMiddlewareRejectsUnauthorizedChatID(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	logger := &MockLogger{}
	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", logger)
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	handlerCalled := false
	mw := bot.AuthMiddleware()
	protectedHandler := mw(func(c tb.Context) error {
		handlerCalled = true
		return nil
	})

	// 1. Unauthorized ChatID & Sender must be rejected
	unauthCtx := newFakeContext(99999999, 88888888, "/start")
	if err := protectedHandler(unauthCtx); err != nil {
		t.Fatalf("Unexpected error from AuthMiddleware: %v", err)
	}
	if handlerCalled {
		t.Fatal("Protected handler must NOT be executed for unauthorized ChatID")
	}
	if len(unauthCtx.sentMsgs) != 1 || unauthCtx.sentMsgs[0] != "⛔ Unauthorized access." {
		t.Fatalf("Expected unauthorized response '⛔ Unauthorized access.', got %v", unauthCtx.sentMsgs)
	}

	// 2. Authorized ChatID must pass through to handler
	authCtx := newFakeContext(12345678, 111, "/start")
	if err := protectedHandler(authCtx); err != nil {
		t.Fatalf("Unexpected error for authorized ChatID: %v", err)
	}
	if !handlerCalled {
		t.Fatal("Protected handler must be executed for authorized ChatID")
	}
}
