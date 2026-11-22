package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	tb "gopkg.in/telebot.v3"
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
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
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
	mu          sync.Mutex
	LastCmdType int32
	LastPayload string
	LastInput   string
	Calls       int
	Refreshes   int
}

func (m *MockPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastCmdType = cmdType
	m.LastPayload = payload
	m.LastInput = input
	m.Calls++
	return nil
}

func (m *MockPublisher) RequestRefresh(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Refreshes++
	return nil
}

func (m *MockPublisher) Close() error { return nil }

type fakeUIContext struct {
	tb.Context
	chatID   int64
	userID   int64
	text     string
	sentMsgs []string
}

func newFakeUIContext(chatID, userID int64, text string) *fakeUIContext {
	return &fakeUIContext{
		chatID: chatID,
		userID: userID,
		text:   text,
	}
}

func (f *fakeUIContext) Chat() *tb.Chat {
	return &tb.Chat{ID: f.chatID}
}

func (f *fakeUIContext) Sender() *tb.User {
	if f.userID == 0 {
		return nil
	}
	return &tb.User{ID: f.userID}
}

func (f *fakeUIContext) Message() *tb.Message {
	return &tb.Message{
		Text:   f.text,
		Chat:   f.Chat(),
		Sender: f.Sender(),
	}
}

func (f *fakeUIContext) Text() string {
	return f.text
}

func (f *fakeUIContext) Send(what interface{}, opts ...interface{}) error {
	if s, ok := what.(string); ok {
		f.sentMsgs = append(f.sentMsgs, s)
	}
	return nil
}

// -----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------

func TestOnComponentConnected_MalformedJSONPreservesExistingMenu(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	clientID := "svc-1"
	validJSON := `[{"label": "🚀 Run", "cmd_type": 10, "payload": "ok"}]`
	validPub := &MockPublisher{}

	OnComponentConnected(bot, clientID, "AlphaService", validJSON, validPub)

	// Send malformed JSON for the same component/clientID
	badPub := &MockPublisher{}
	OnComponentConnected(bot, clientID, "AlphaService", `{not-valid-json`, badPub)

	bot.Mu.RLock()
	menu, hasMenu := bot.DynamicMenus[clientID]
	activePub := bot.Publishers[clientID]
	bot.Mu.RUnlock()

	if !hasMenu || menu == nil {
		t.Fatal("Existing valid menu was destroyed by malformed JSON update")
	}
	if len(menu.Root.Rows) != 1 || menu.Root.Rows[0].Buttons[0].Label != "🚀 Run" {
		t.Fatalf("Existing menu contents were corrupted: %+v", menu.Root)
	}
	if activePub != validPub {
		t.Fatal("Existing valid publisher was overwritten on malformed JSON update")
	}
}

// -----------------------------------------------------------------------------

func TestOnDisconnect_StaleStreamDoesNotDeleteNewSession(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	clientID := "svc-reconnect:127.0.0.1:9000"
	oldPub := &MockPublisher{}
	newPub := &MockPublisher{}

	// 1. Old stream registers
	OnComponentConnected(bot, clientID, "ReconnectService", `[{"label": "V1", "cmd_type": 1}]`, oldPub)

	// 2. New stream reconnects for the same clientID before old stream disconnect fires
	OnComponentConnected(bot, clientID, "ReconnectService", `[{"label": "V2", "cmd_type": 2}]`, newPub)

	convKey := core.ConversationKey{ChatID: 12345678, UserID: 42}
	bot.Mu.Lock()
	bot.UserStates[convKey] = clientID
	bot.UserPaths[convKey] = []string{"Sub"}
	bot.Mu.Unlock()

	// 3. Old stream closes and calls OnDisconnect with oldPub
	bot.OnDisconnect(clientID, oldPub)

	bot.Mu.RLock()
	menu, hasMenu := bot.DynamicMenus[clientID]
	activePub := bot.Publishers[clientID]
	_, hasUserState := bot.UserStates[convKey]
	bot.Mu.RUnlock()

	if !hasMenu || menu == nil {
		t.Fatal("Stale stream OnDisconnect deleted the newly reconnected menu session")
	}
	if activePub != newPub {
		t.Fatal("Stale stream OnDisconnect altered the active publisher")
	}
	if !hasUserState {
		t.Fatal("Stale stream OnDisconnect cleared UserStates for active session")
	}

	// 4. Now the active newPub disconnects -> session and user states must be cleaned up
	bot.OnDisconnect(clientID, newPub)

	bot.Mu.RLock()
	_, hasMenuAfter := bot.DynamicMenus[clientID]
	_, hasPubAfter := bot.Publishers[clientID]
	_, hasUserStateAfter := bot.UserStates[convKey]
	_, hasUserPathAfter := bot.UserPaths[convKey]
	bot.Mu.RUnlock()

	if hasMenuAfter || hasPubAfter {
		t.Fatal("Active stream OnDisconnect failed to remove component session")
	}
	if hasUserStateAfter || hasUserPathAfter {
		t.Fatal("Active stream OnDisconnect failed to clean up UserStates/UserPaths pointing to disconnected clientID")
	}
}

// -----------------------------------------------------------------------------

func TestTwoStepConfirmation_EmergencyCommands(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	pub1 := &MockPublisher{}
	pub2 := &MockPublisher{}
	OnComponentConnected(bot, "c1", "Svc1", `[{"label": "Ping", "cmd_type": 9}]`, pub1)
	OnComponentConnected(bot, "c2", "Svc2", `[{"label": "Ping", "cmd_type": 9}]`, pub2)

	// 1. Trigger "🆘 power off !" -> must NOT broadcast immediately, must record PendingConfirmation
	step1Ctx := newFakeUIContext(12345678, 777, models.BtnPowerOff)
	if err := HandleDynamicText(bot, step1Ctx); err != nil {
		t.Fatalf("HandleDynamicText failed on power off trigger: %v", err)
	}
	if pub1.Calls != 0 || pub2.Calls != 0 {
		t.Fatalf("Power off must not broadcast before 2-step confirmation (calls: %d, %d)", pub1.Calls, pub2.Calls)
	}

	// 2. Abort with "❌ Cancel" -> must clear confirmation without broadcasting
	cancelCtx := newFakeUIContext(12345678, 777, models.BtnCancel)
	if err := HandleDynamicText(bot, cancelCtx); err != nil {
		t.Fatalf("HandleDynamicText failed on cancel: %v", err)
	}
	if pub1.Calls != 0 || pub2.Calls != 0 {
		t.Fatal("Cancel must not broadcast commands")
	}

	// Clicking "✅ CONFIRM POWER OFF" after cancel must NOT execute
	lateConfirmCtx := newFakeUIContext(12345678, 777, models.BtnConfirmPowerOff)
	if err := HandleDynamicText(bot, lateConfirmCtx); err != nil {
		t.Fatalf("HandleDynamicText failed on late confirm: %v", err)
	}
	if pub1.Calls != 0 || pub2.Calls != 0 {
		t.Fatal("Confirm after cancel must not broadcast commands")
	}

	// 3. Trigger "🆘 power off !" again and confirm with "✅ CONFIRM POWER OFF"
	if err := RequestEmergencyConfirmation(bot, step1Ctx, models.CmdPowerOff); err != nil {
		t.Fatalf("RequestEmergencyConfirmation failed: %v", err)
	}
	confirmPowerCtx := newFakeUIContext(12345678, 777, models.BtnConfirmPowerOff)
	if err := HandleDynamicText(bot, confirmPowerCtx); err != nil {
		t.Fatalf("HandleDynamicText failed on confirm power off: %v", err)
	}
	if pub1.Calls != 1 || pub1.LastCmdType != models.CmdPowerOff || pub2.Calls != 1 || pub2.LastCmdType != models.CmdPowerOff {
		t.Fatalf("Expected CmdPowerOff broadcast to both publishers, got pub1=(%d, type=%d), pub2=(%d, type=%d)",
			pub1.Calls, pub1.LastCmdType, pub2.Calls, pub2.LastCmdType)
	}

	// 4. Trigger "⏏️ close all positions" and confirm with "✅ CONFIRM CLOSE ALL POSITIONS"
	closeStep1Ctx := newFakeUIContext(12345678, 777, models.BtnCloseAll)
	if err := HandleDynamicText(bot, closeStep1Ctx); err != nil {
		t.Fatalf("HandleDynamicText failed on close all trigger: %v", err)
	}
	if pub1.Calls != 1 || pub2.Calls != 1 {
		t.Fatal("Close all positions must not broadcast before confirmation")
	}

	confirmCloseCtx := newFakeUIContext(12345678, 777, models.BtnConfirmCloseAll)
	if err := HandleDynamicText(bot, confirmCloseCtx); err != nil {
		t.Fatalf("HandleDynamicText failed on confirm close all: %v", err)
	}
	if pub1.Calls != 2 || pub1.LastCmdType != models.CmdStop || pub2.Calls != 2 || pub2.LastCmdType != models.CmdStop {
		t.Fatalf("Expected CmdStop broadcast to both publishers, got pub1=(%d, type=%d), pub2=(%d, type=%d)",
			pub1.Calls, pub1.LastCmdType, pub2.Calls, pub2.LastCmdType)
	}
}

// -----------------------------------------------------------------------------

func TestPendingInput_RoutesToOriginatingClientAndIsolatesUsers(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	pubA := &MockPublisher{}
	pubB := &MockPublisher{}

	menuA := `[{"label": "Set Host", "cmd_type": 11, "payload": "host_key", "input_prompt": "Enter new host for A:"}]`
	menuB := `[{"label": "Set Port", "cmd_type": 22, "payload": "port_key", "input_prompt": "Enter new port for B:"}]`

	OnComponentConnected(bot, "client-A", "ServiceA", menuA, pubA)
	OnComponentConnected(bot, "client-B", "ServiceB", menuB, pubB)

	chatID := int64(12345678)
	user1 := int64(101)
	user2 := int64(202)

	// 1. User 1 opens ServiceA and clicks "Set Host"
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user1, "📂 ServiceA")); err != nil {
		t.Fatalf("User 1 failed to enter ServiceA: %v", err)
	}
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user1, "Set Host")); err != nil {
		t.Fatalf("User 1 failed to trigger input prompt on ServiceA: %v", err)
	}

	// 2. User 2 in the SAME chat opens ServiceB and clicks "Set Port"
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user2, "📂 ServiceB")); err != nil {
		t.Fatalf("User 2 failed to enter ServiceB: %v", err)
	}
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user2, "Set Port")); err != nil {
		t.Fatalf("User 2 failed to trigger input prompt on ServiceB: %v", err)
	}

	// 3. Simulate User 1's UserStates changing to "client-B" before User 1 replies!
	convKey1 := core.ConversationKey{ChatID: chatID, UserID: user1}
	bot.Mu.Lock()
	bot.UserStates[convKey1] = "client-B"
	bot.Mu.Unlock()

	// 4. User 1 replies with "10.0.0.42" -> must route to originating "client-A" (pubA), NOT "client-B"
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user1, "10.0.0.42")); err != nil {
		t.Fatalf("User 1 input reply failed: %v", err)
	}
	if pubA.Calls != 1 || pubA.LastCmdType != 11 || pubA.LastPayload != "host_key" || pubA.LastInput != "10.0.0.42" {
		t.Fatalf("Expected User 1 input to route to pubA with '10.0.0.42', got calls=%d cmd=%d payload=%s input=%s",
			pubA.Calls, pubA.LastCmdType, pubA.LastPayload, pubA.LastInput)
	}
	if pubB.Calls != 0 {
		t.Fatalf("User 1 reply must not have been sent to pubB (calls=%d)", pubB.Calls)
	}

	// 5. User 2 replies with "8080" -> must route to "client-B" (pubB)
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, user2, "8080")); err != nil {
		t.Fatalf("User 2 input reply failed: %v", err)
	}
	if pubB.Calls != 1 || pubB.LastCmdType != 22 || pubB.LastPayload != "port_key" || pubB.LastInput != "8080" {
		t.Fatalf("Expected User 2 input to route to pubB with '8080', got calls=%d cmd=%d payload=%s input=%s",
			pubB.Calls, pubB.LastCmdType, pubB.LastPayload, pubB.LastInput)
	}
}
