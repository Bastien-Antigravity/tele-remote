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
	publishers   map[string]tele_interfaces.Publisher
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
		publishers:   make(map[string]tele_interfaces.Publisher),
	}

	// Load initial state if persistence is available
	if pm != nil {
		if state, err := pm.Load(); err == nil && len(state) > 0 {
			bot.log.Info("Restoring component registry from persistence", "count", len(state))
			bot.dynamicMenus = state
			// Note: We don't restore publishers as they require active network connections
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

	// We keep the menu in persistence as requested, but remove the publisher
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
