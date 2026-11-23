package ui

import (
	"context"
	"errors"
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
	PublishErr  error
}

func (m *MockPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastCmdType = cmdType
	m.LastPayload = payload
	m.LastInput = input
	m.Calls++
	return m.PublishErr
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
	chatID      int64
	userID      int64
	text        string
	sentMsgs    []string
	sentMarkups []*tb.ReplyMarkup
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
	for _, opt := range opts {
		if markup, ok := opt.(*tb.ReplyMarkup); ok {
			f.sentMarkups = append(f.sentMarkups, markup)
		}
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
	foundConfirmMsg := false
	for _, msg := range confirmPowerCtx.sentMsgs {
		if strings.Contains(msg, "Power-off command delivered to all 2 connected components") {
			foundConfirmMsg = true
			break
		}
	}
	if !foundConfirmMsg {
		t.Fatalf("Expected delivery confirmation message, got %v", confirmPowerCtx.sentMsgs)
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

// -----------------------------------------------------------------------------

func TestEmergencyBroadcast_PartialFailureReporting(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	pubOK := &MockPublisher{}
	pubFail := &MockPublisher{PublishErr: errors.New("stream closed")}

	OnComponentConnected(bot, "c1", "AlphaSvc", `[{"label": "Ping", "cmd_type": 9}]`, pubOK)
	OnComponentConnected(bot, "c2", "BetaSvc", `[{"label": "Ping", "cmd_type": 9}]`, pubFail)

	step1Ctx := newFakeUIContext(12345678, 777, models.BtnPowerOff)
	if err := RequestEmergencyConfirmation(bot, step1Ctx, models.CmdPowerOff); err != nil {
		t.Fatalf("RequestEmergencyConfirmation failed: %v", err)
	}

	confirmCtx := newFakeUIContext(12345678, 777, models.BtnConfirmPowerOff)
	if err := HandleDynamicText(bot, confirmCtx); err != nil {
		t.Fatalf("HandleDynamicText failed: %v", err)
	}

	foundReport := false
	for _, msg := range confirmCtx.sentMsgs {
		if strings.Contains(msg, "⚠️ Power-off delivered to 1/2 components") && strings.Contains(msg, "BetaSvc (stream closed)") {
			foundReport = true
			break
		}
	}
	if !foundReport {
		t.Fatalf("Expected partial failure report message, got %v", confirmCtx.sentMsgs)
	}
}

// -----------------------------------------------------------------------------

func TestOnComponentConnected_SameClientIDInvalidatesPendingInput(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	clientID := "svc-same-id"
	pub1 := &MockPublisher{}
	menuV1 := `[{"label": "Set Value", "cmd_type": 10, "payload": "val", "input_prompt": "Enter value:"}]`
	OnComponentConnected(bot, clientID, "SameService", menuV1, pub1)

	chatID := int64(12345678)
	userID := int64(99)
	convKey := core.ConversationKey{ChatID: chatID, UserID: userID}

	if err := HandleDynamicText(bot, newFakeUIContext(chatID, userID, "📂 SameService")); err != nil {
		t.Fatalf("Failed to enter SameService: %v", err)
	}
	if err := HandleDynamicText(bot, newFakeUIContext(chatID, userID, "Set Value")); err != nil {
		t.Fatalf("Failed to trigger input prompt: %v", err)
	}

	bot.Mu.RLock()
	pendingBefore, hasPendingBefore := bot.PendingInputs[convKey]
	bot.Mu.RUnlock()
	if !hasPendingBefore || pendingBefore.MenuRevision != 1 {
		t.Fatalf("Expected pending input with MenuRevision=1 before reconnect, got exists=%v rev=%d", hasPendingBefore, pendingBefore.MenuRevision)
	}

	pub2 := &MockPublisher{}
	menuV2 := `[{"label": "Updated Action", "cmd_type": 20}]`
	OnComponentConnected(bot, clientID, "SameService", menuV2, pub2)

	bot.Mu.RLock()
	_, hasPendingAfter := bot.PendingInputs[convKey]
	comp := bot.DynamicMenus[clientID]
	bot.Mu.RUnlock()

	if hasPendingAfter {
		t.Fatal("Expected PendingInputs for same clientID to be cleared on reconnect")
	}
	if comp == nil || comp.Revision != 2 {
		t.Fatalf("Expected comp.Revision == 2, got %+v", comp)
	}
}

// -----------------------------------------------------------------------------

func TestOnComponentConnected_RejectsOversizedAndDeepMenus(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	clientID := "svc-guarded"
	validPub := &MockPublisher{}
	validJSON := `[{"label": "Valid Button", "cmd_type": 1}]`
	OnComponentConnected(bot, clientID, "GuardedSvc", validJSON, validPub)

	// 1. Oversized (> 64 KB) JSON
	oversizedPayload := strings.Repeat("a", maxMenuJSONBytes+100)
	oversizedJSON := `[{"label": "` + oversizedPayload + `", "cmd_type": 2}]`
	OnComponentConnected(bot, clientID, "GuardedSvc", oversizedJSON, &MockPublisher{})

	bot.Mu.RLock()
	menuAfterOversized := bot.DynamicMenus[clientID]
	pubAfterOversized := bot.Publishers[clientID]
	bot.Mu.RUnlock()

	if menuAfterOversized == nil || menuAfterOversized.Root.Rows[0].Buttons[0].Label != "Valid Button" || pubAfterOversized != validPub {
		t.Fatal("Oversized menu JSON overwrote or corrupted existing valid menu")
	}

	// 2. Deeply nested (> maxMenuDepth) JSON
	deepJSON := `{"label": "Leaf", "cmd_type": 3}`
	for i := 0; i < maxMenuDepth+2; i++ {
		deepJSON = `{"label": "Level", "menu": [` + deepJSON + `]}`
	}
	deepJSON = `[` + deepJSON + `]`
	OnComponentConnected(bot, clientID, "GuardedSvc", deepJSON, &MockPublisher{})

	bot.Mu.RLock()
	menuAfterDeep := bot.DynamicMenus[clientID]
	pubAfterDeep := bot.Publishers[clientID]
	bot.Mu.RUnlock()

	if menuAfterDeep == nil || menuAfterDeep.Root.Rows[0].Buttons[0].Label != "Valid Button" || pubAfterDeep != validPub {
		t.Fatal("Deeply nested menu JSON overwrote or corrupted existing valid menu")
	}
}

// -----------------------------------------------------------------------------

func TestShowMainMenu_DeterministicAlphabeticalOrder(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	OnComponentConnected(bot, "id-zeta", "Zeta Service", `[{"label": "Ping", "cmd_type": 1}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-alpha", "Alpha Service", `[{"label": "Ping", "cmd_type": 1}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-beta", "Beta Config", `{"priority": 100, "rows": [{"label": "Ping", "cmd_type": 1}]}`, &MockPublisher{})

	ctx := newFakeUIContext(12345678, 777, "")
	if err := ShowMainMenu(bot, ctx); err != nil {
		t.Fatalf("ShowMainMenu failed: %v", err)
	}

	if len(ctx.sentMarkups) != 1 {
		t.Fatalf("Expected 1 ReplyMarkup sent, got %d", len(ctx.sentMarkups))
	}
	rows := ctx.sentMarkups[0].ReplyKeyboard
	if len(rows) < 4 {
		t.Fatalf("Expected at least 4 keyboard rows, got %d", len(rows))
	}

	expected := []string{"📂 Alpha Service", "⚙️ Beta Config", "📂 Zeta Service"}
	for i, exp := range expected {
		if len(rows[i]) != 1 || rows[i][0].Text != exp {
			t.Fatalf("Row %d expected %q, got %+v", i, exp, rows[i])
		}
	}
}

// -----------------------------------------------------------------------------

func TestShowMainMenu_OpenMFEPriorityAndDuplicateOrderTolerance(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	// Mix default catalog priorities, wrapper JSON object priorities, metadata-only array header,
	// component_priority field, and explicit duplicate priorities (both priority 20 and default 100).
	OnComponentConnected(bot, "id-custom-z", "Zeta Worker", `[{"label": "Ping", "cmd_type": 1}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-custom-a", "Alpha Worker", `[{"label": "Ping", "cmd_type": 1}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-explicit-50", "Metrics Engine", `[{"component_priority": 50, "label": "Stats", "cmd_type": 2}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-dup-20-b", "Trading Desk", `[{"priority": 20}, {"label": "Status", "cmd_type": 3}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-config", "Config Server", `[{"label": "Reload", "cmd_type": 4}]`, &MockPublisher{})
	OnComponentConnected(bot, "id-squad", "Squad Manager", `{"order": "10", "menu": [{"label": "List", "cmd_type": 5}]}`, &MockPublisher{})
	OnComponentConnected(bot, "id-web", "Web UI", `{"priority": 5, "buttons": [{"label": "Health", "cmd_type": 6}]}`, &MockPublisher{})

	// Verify metadata-only header row was NOT added as a button row for Trading Desk.
	bot.Mu.RLock()
	tradingMenu := bot.DynamicMenus["id-dup-20-b"]
	metricsMenu := bot.DynamicMenus["id-explicit-50"]
	bot.Mu.RUnlock()
	if tradingMenu == nil || len(tradingMenu.Root.Rows) != 1 || tradingMenu.Root.Rows[0].Buttons[0].Label != "Status" {
		t.Fatalf("Expected metadata-only row to be skipped for Trading Desk, got %+v", tradingMenu)
	}
	if metricsMenu == nil || metricsMenu.Priority != 50 || len(metricsMenu.Root.Rows) != 1 {
		t.Fatalf("Expected component_priority=50 and 1 row for Metrics Engine, got %+v", metricsMenu)
	}

	ctx := newFakeUIContext(12345678, 777, "")
	if err := ShowMainMenu(bot, ctx); err != nil {
		t.Fatalf("ShowMainMenu failed: %v", err)
	}

	if len(ctx.sentMarkups) != 1 {
		t.Fatalf("Expected 1 ReplyMarkup sent, got %d", len(ctx.sentMarkups))
	}
	rows := ctx.sentMarkups[0].ReplyKeyboard
	expected := []string{
		"📂 Web UI",         // priority 5
		"📂 Squad Manager",  // priority 10
		"⚙️ Config Server",  // default priority 20
		"📂 Trading Desk",   // duplicate priority 20 ("Config Server" < "Trading Desk")
		"📂 Metrics Engine", // explicit component_priority 50
		"📂 Alpha Worker",   // default priority 100
		"📂 Zeta Worker",    // duplicate default priority 100 ("Alpha Worker" < "Zeta Worker")
	}
	if len(rows) != len(expected)+1 {
		t.Fatalf("Expected %d keyboard rows (including footer), got %d", len(expected)+1, len(rows))
	}
	for i, exp := range expected {
		if len(rows[i]) != 1 || rows[i][0].Text != exp {
			t.Fatalf("Row %d expected %q, got %+v", i, exp, rows[i])
		}
	}
}

// -----------------------------------------------------------------------------

func TestMenu_RowAndButtonOrderingWithDuplicateTolerance(t *testing.T) {
	ms := newMockTelegramServer()
	defer ms.Close()

	bot, err := core.NewBot("12345:TEST_TOKEN", ms.server.URL, "12345678", &MockLogger{})
	if err != nil {
		t.Fatalf("Failed to init bot: %v", err)
	}

	menuJSON := `{
		"priority": 15,
		"rows": [
			{"label": "Unordered Tail", "cmd_type": 9},
			{"label": "Dup Second A", "order": 2, "cmd_type": 2},
			{"order": 1, "buttons": [
				{"label": "Inner Unordered", "cmd_type": 10},
				{"label": "Inner Dup A", "order": 2, "cmd_type": 11},
				{"label": "Inner First", "order": "1", "cmd_type": 12},
				{"label": "Inner Dup B", "order": 2, "cmd_type": 13}
			]},
			{"label": "Dup Second B", "priority": 2, "cmd_type": 3},
			{"label": "Submenu", "id": 3, "menu": [
				{"label": "Sub Unordered", "cmd_type": 20},
				{"label": "Sub Dup A", "order": 2, "cmd_type": 21},
				{"label": "Sub First", "order": 1, "cmd_type": 22},
				{"label": "Sub Dup B", "order": 2, "cmd_type": 23}
			]}
		]
	}`

	OnComponentConnected(bot, "id-ordered", "Watchdog Service", menuJSON, &MockPublisher{})

	bot.Mu.RLock()
	comp := bot.DynamicMenus["id-ordered"]
	bot.Mu.RUnlock()

	if comp == nil || comp.Priority != 15 {
		t.Fatalf("Expected Priority=15, got %+v", comp)
	}

	// Render root menu and check exact order (duplicate order 2 preserves declaration order A before B; unordered goes last)
	rootCtx := newFakeUIContext(12345678, 777, "")
	if err := RenderMenu(bot, rootCtx, comp.Root); err != nil {
		t.Fatalf("RenderMenu failed: %v", err)
	}
	if len(rootCtx.sentMarkups) != 1 {
		t.Fatalf("Expected 1 ReplyMarkup, got %d", len(rootCtx.sentMarkups))
	}
	rootRows := rootCtx.sentMarkups[0].ReplyKeyboard
	expectedRootLabels := []string{
		"Inner First",     // row order 1, btn order 1
		"Inner Dup A",     // row order 1, btn order 2 (declared first)
		"Inner Dup B",     // row order 1, btn order 2 (declared second)
		"Inner Unordered", // row order 1, btn unordered (1000)
		"Dup Second A",    // row order 2 (declared first)
		"Dup Second B",    // row order 2 (declared second)
		"Submenu",         // row order 3
		"Unordered Tail",  // row unordered (1000)
	}
	if len(rootRows) != len(expectedRootLabels)+1 {
		t.Fatalf("Expected %d root rows (with footer), got %d", len(expectedRootLabels)+1, len(rootRows))
	}
	for i, exp := range expectedRootLabels {
		if rootRows[i][0].Text != exp {
			t.Fatalf("Root row %d expected %q, got %q", i, exp, rootRows[i][0].Text)
		}
	}

	// Check submenu ordering too
	subMenu := getMenuByPath(comp.Root, []string{"Submenu"})
	if subMenu == nil {
		t.Fatal("Expected Submenu to be resolvable")
	}
	subCtx := newFakeUIContext(12345678, 777, "")
	if err := RenderMenu(bot, subCtx, subMenu); err != nil {
		t.Fatalf("RenderMenu submenu failed: %v", err)
	}
	subRows := subCtx.sentMarkups[0].ReplyKeyboard
	expectedSubLabels := []string{"Sub First", "Sub Dup A", "Sub Dup B", "Sub Unordered"}
	if len(subRows) != len(expectedSubLabels)+1 {
		t.Fatalf("Expected %d submenu rows (with footer), got %d", len(expectedSubLabels)+1, len(subRows))
	}
	for i, exp := range expectedSubLabels {
		if subRows[i][0].Text != exp {
			t.Fatalf("Submenu row %d expected %q, got %q", i, exp, subRows[i][0].Text)
		}
	}
}
