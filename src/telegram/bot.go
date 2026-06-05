package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/config"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/store"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------
// Bot Definition
// -----------------------------------------------------------------------------

// Bot holds the telegram connection, config, and state references
type Bot struct {
	b       *tb.Bot
	log     unilog_ifaces.Logger
	cfg     *config.Config
	pm      *store.PersistenceManager

	Menus map[string]*models.CommandMenu

	mu           sync.RWMutex
	dynamicMenus map[string]*models.ComponentMenu
	actionMap    map[string]models.CallbackAction
	cbCounter    int
	publishers   map[string]tele_interfaces.IPublisher
}

// -----------------------------------------------------------------------------
// Factory
// -----------------------------------------------------------------------------

// NewBot registers Telebot settings and initializes memory maps
func NewBot(cfg *config.Config, log unilog_ifaces.Logger, pm *store.PersistenceManager) (*Bot, error) {
	pref := tb.Settings{
		URL:    cfg.TelegramURL,
		Token:  cfg.TelegramToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tb.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}

	bot := &Bot{
		b:            b,
		log:          log,
		cfg:          cfg,
		pm:           pm,
		Menus:        make(map[string]*models.CommandMenu),
		dynamicMenus: make(map[string]*models.ComponentMenu),
		actionMap:    make(map[string]models.CallbackAction),
		publishers:   make(map[string]tele_interfaces.IPublisher),
	}

	// Load initial state if persistence is available
	if pm != nil {
		if state, err := pm.Load(); err == nil && len(state) > 0 {
			bot.log.Info("Restoring component registry from persistence", "count", len(state))
			bot.dynamicMenus = state
			// Restore actions for all buttons in the persisted menus
			for clientID, comp := range state {
				bot.restoreMenuActions(comp.Root, clientID)
			}
		}
	}

	return bot, nil
}

// -----------------------------------------------------------------------------
// Lifecycle
// -----------------------------------------------------------------------------

// Start triggers routing setup, background listeners, and begins polling
func (bot *Bot) Start(ctx context.Context) {
	bot.setupRoutes()

	go func() {
		<-ctx.Done()
		bot.log.Info("Shutting down Telebot gracefully...")
		bot.b.Stop()
	}()

	bot.log.Info("Telebot starting polling...", "chatID", bot.cfg.ChatID)
	bot.b.Start()
}

// -----------------------------------------------------------------------------
// Telemetry & Output
// -----------------------------------------------------------------------------

// Broadcast sends a plain text message to the pre-configured ChatID
func (bot *Bot) Broadcast(msg string) {
	if bot.cfg.ChatID == "" {
		bot.log.Warning("Broadcast failed: TB_CHATID not set")
		return
	}

	var chatID int64
	fmt.Sscanf(bot.cfg.ChatID, "%d", &chatID)
	chat := &tb.Chat{ID: chatID}

	_, err := bot.b.Send(chat, msg)
	if err != nil {
		bot.log.Error("failed to broadcast telemetry", "err", err)
	}
}

// OnTelemetry handles incoming logs or events by broadcasting them to the admin chat
func (bot *Bot) OnTelemetry(msg string) {
	bot.Broadcast(msg)
}

// -----------------------------------------------------------------------------
// Component Handlers
// -----------------------------------------------------------------------------

// OnDisconnect cleans up a component's state when it loses connection
func (bot *Bot) OnDisconnect(clientID string) {
	bot.mu.Lock()
	defer bot.mu.Unlock()

	// We keep the menu in persistence, but remove the publisher
	if _, ok := bot.dynamicMenus[clientID]; ok {
		bot.log.Info("Removing publisher for disconnected component (Menu remains)", "client", clientID)
		delete(bot.publishers, clientID)
	}
}

// SaveState flushes the current registry to disk
func (bot *Bot) SaveState() error {
	if bot.pm == nil {
		return nil
	}
	bot.mu.RLock()
	defer bot.mu.RUnlock()
	return bot.pm.Save(bot.dynamicMenus)
}

// -----------------------------------------------------------------------------
// Internal Logic
// -----------------------------------------------------------------------------

// restoreMenuActions recursively re-registers callback actions for restored menus
func (bot *Bot) restoreMenuActions(m *models.CommandMenu, clientID string) {
	if m == nil {
		return
	}
	for i := range m.Rows {
		for j := range m.Rows[i].Buttons {
			btn := &m.Rows[i].Buttons[j]
			if btn.NextMenu != nil {
				bot.restoreMenuActions(btn.NextMenu, clientID)
			} else if btn.CallbackData != "" {
				// Re-register the action using the same callback ID
				bot.registerActionWithID(btn.CallbackData, bot.createCommandAction(clientID, btn.CommandType, btn.Payload, btn.Label))
			}
		}
	}
}

func (bot *Bot) createCommandAction(clientID string, cmdType int32, payload, label string) models.CallbackAction {
	return func(ctx tb.Context) error {
		bot.mu.RLock()
		pub, ok := bot.publishers[clientID]
		bot.mu.RUnlock()

		if !ok {
			return ctx.Send("❌ Component disconnected.")
		}

		bot.log.Info("Executing component command", "client", clientID, "type", cmdType)
		
		// Use a context with timeout for command dispatch
		dispatchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := pub.PublishCommand(dispatchCtx, cmdType, payload); err != nil {
			return ctx.Send(fmt.Sprintf("⚠️ Failed to send command: %v", err))
		}
		return ctx.Send(fmt.Sprintf("✅ Sent: %s", label))
	}
}

func (bot *Bot) registerActionWithID(id string, fn models.CallbackAction) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.actionMap[id] = fn
}
