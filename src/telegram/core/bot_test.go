package core

import (
	"context"
	"encoding/json"
	"fmt"
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
	infos    []string
}

func (m *MockLogger) Debug(f string, a ...any) {}
func (m *MockLogger) Info(f string, a ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infos = append(m.infos, fmt.Sprintf("%s %v", f, a))
}
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
	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678, 87654321, -10099999", logger, "99990000")
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	handlerCalled := false
	mw := bot.AuthMiddleware()
	protectedHandler := mw(func(c tb.Context) error {
		handlerCalled = true
		return nil
	})

	// 1. Unauthorized ChatID & Sender must be rejected with "⛔ Unauthorized access."
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

	// 2. Authorized private user (chatID=12345678, userID=12345678) must pass
	handlerCalled = false
	authCtx1 := newFakeContext(12345678, 12345678, "/start")
	if err := protectedHandler(authCtx1); err != nil {
		t.Fatalf("Unexpected error for authorized private user: %v", err)
	}
	if !handlerCalled {
		t.Fatal("Protected handler must be executed for authorized private user 12345678")
	}

	// 3. Authorized sender in a group (chatID=-10055555, userID=87654321) must pass
	handlerCalled = false
	authCtx2 := newFakeContext(-10055555, 87654321, "/start")
	if err := protectedHandler(authCtx2); err != nil {
		t.Fatalf("Unexpected error for authorized group sender: %v", err)
	}
	if !handlerCalled {
		t.Fatal("Protected handler must be executed for authorized Sender 87654321 in group -10055555")
	}

	// 4. Unauthorized sender (userID=111) inside an authorized group chat (chatID=-10099999) must be REJECTED
	handlerCalled = false
	unauthGroupCtx := newFakeContext(-10099999, 111, "/start")
	if err := protectedHandler(unauthGroupCtx); err != nil {
		t.Fatalf("Unexpected error for unauthorized group sender: %v", err)
	}
	if handlerCalled {
		t.Fatal("Protected handler must NOT be executed for unauthorized sender inside authorized group chat")
	}
	if len(unauthGroupCtx.sentMsgs) != 1 || unauthGroupCtx.sentMsgs[0] != "⛔ Unauthorized access." {
		t.Fatalf("Expected unauthorized response '⛔ Unauthorized access.', got %v", unauthGroupCtx.sentMsgs)
	}

	// 5. Authorized private chat when userID=0 (Sender() == nil, chatID=12345678) must pass
	handlerCalled = false
	authNilSenderCtx := newFakeContext(12345678, 0, "/start")
	if err := protectedHandler(authNilSenderCtx); err != nil {
		t.Fatalf("Unexpected error for authorized private chat with nil Sender: %v", err)
	}
	if !handlerCalled {
		t.Fatal("Protected handler must be executed for authorized private chat 12345678 with nil Sender")
	}

	// 6. Authorized ID from AllowedUsers
	handlerCalled = false
	authCtx3 := newFakeContext(999, 99990000, "/start")
	if err := protectedHandler(authCtx3); err != nil {
		t.Fatalf("Unexpected error for allowed user: %v", err)
	}
	if !handlerCalled {
		t.Fatal("Protected handler must be executed for AllowedUsers ID 99990000")
	}
}

// -----------------------------------------------------------------------------

func TestBot_RateLimitMiddleware(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	logger := &MockLogger{}
	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", logger)
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}
	bot.Limiter = NewRateLimiter(2, 500*time.Millisecond, 1*time.Second)

	callCount := 0
	rlMw := bot.RateLimitMiddleware()
	handler := rlMw(func(c tb.Context) error {
		callCount++
		return nil
	})

	ctx := newFakeContext(12345678, 12345678, "hello")

	// 1st request -> allowed
	if err := handler(ctx); err != nil {
		t.Fatalf("Request 1 error: %v", err)
	}
	// 2nd request -> allowed
	if err := handler(ctx); err != nil {
		t.Fatalf("Request 2 error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("Expected callCount=2, got %d", callCount)
	}

	// 3rd request -> throttled, sends warning
	if err := handler(ctx); err != nil {
		t.Fatalf("Request 3 error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("Throttled request should not increment handler callCount, got %d", callCount)
	}
	if len(ctx.sentMsgs) == 0 || !strings.Contains(ctx.sentMsgs[0], "Slow down") {
		t.Fatalf("Expected 'Slow down' warning message, got %v", ctx.sentMsgs)
	}

	// 4th request -> throttled, silent (cooldown)
	msgCountBefore := len(ctx.sentMsgs)
	if err := handler(ctx); err != nil {
		t.Fatalf("Request 4 error: %v", err)
	}
	if len(ctx.sentMsgs) != msgCountBefore {
		t.Fatalf("Flooded request in cooldown should not send another warning message")
	}
}

// -----------------------------------------------------------------------------

func TestBot_LoggingMiddlewareRedactsPendingInput(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	logger := &MockLogger{}
	bot, err := NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", logger)
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	convKey := ConversationKey{ChatID: 12345678, UserID: 12345678}
	bot.Mu.Lock()
	bot.PendingInputs[convKey] = PendingInput{ClientID: "auth-service"}
	bot.Mu.Unlock()

	mw := bot.LoggingMiddleware()
	handler := mw(func(c tb.Context) error { return nil })

	secretText := "super-secret-api-key-123"
	if err := handler(newFakeContext(12345678, 12345678, secretText)); err != nil {
		t.Fatalf("Unexpected error from LoggingMiddleware (pending): %v", err)
	}

	bot.Mu.Lock()
	delete(bot.PendingInputs, convKey)
	bot.Mu.Unlock()

	if err := handler(newFakeContext(12345678, 12345678, "/status")); err != nil {
		t.Fatalf("Unexpected error from LoggingMiddleware (normal): %v", err)
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.infos) != 2 {
		t.Fatalf("Expected 2 info logs, got %d", len(logger.infos))
	}
	if strings.Contains(logger.infos[0], secretText) {
		t.Fatalf("Pending input secret must NOT be logged, got %q", logger.infos[0])
	}
	if !strings.Contains(logger.infos[0], "[REDACTED_PENDING_INPUT]") {
		t.Fatalf("Expected [REDACTED_PENDING_INPUT] in log, got %q", logger.infos[0])
	}
	if !strings.Contains(logger.infos[1], "/status") {
		t.Fatalf("Expected normal command '/status' in log, got %q", logger.infos[1])
	}
}

// -----------------------------------------------------------------------------

func TestSplitTelegramMessage_ChunksLongMessages(t *testing.T) {
	if got := splitTelegramMessage("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("Expected single empty string for empty input, got %v", got)
	}

	if got := splitTelegramMessage("short", 10); len(got) != 1 || got[0] != "short" {
		t.Fatalf("Expected single chunk for short input, got %v", got)
	}

	// Should split on newline when newline is after maxLen/2
	nlMsg := "line12\nline2-ext"
	nlChunks := splitTelegramMessage(nlMsg, 10)
	if len(nlChunks) != 2 || nlChunks[0] != "line12\n" || nlChunks[1] != "line2-ext" {
		t.Fatalf("Expected newline-aligned split [\"line12\\n\", \"line2-ext\"], got %q", nlChunks)
	}

	// Long message without newlines should split cleanly at maxLen runes
	longMsg := strings.Repeat("a", maxTelegramMessageLen*2+123)
	chunks := splitTelegramMessage(longMsg, maxTelegramMessageLen)
	if len(chunks) != 3 {
		t.Fatalf("Expected 3 chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if len([]rune(chunk)) > maxTelegramMessageLen {
			t.Fatalf("Chunk %d exceeded max length %d: got %d", i, maxTelegramMessageLen, len([]rune(chunk)))
		}
	}
	if strings.Join(chunks, "") != longMsg {
		t.Fatal("Rejoined chunks do not match original message")
	}
}
