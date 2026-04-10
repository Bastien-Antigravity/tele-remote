package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/config"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

// Bot holds the telegram connection, config, and state references
type Bot struct {
	b       *tb.Bot
	log     interfaces.Logger
	cfg     *config.Config

	Menus map[string]*CommandMenu

	mu           sync.RWMutex
	dynamicMenus map[string]*ComponentMenu
	actionMap    map[string]CallbackAction
	cbCounter    int
	publishers   map[string]tele_interfaces.Publisher
}

// -----------------------------------------------------------------------------

// NewBot registers Telebot settings and initializes memory maps
func NewBot(cfg *config.Config, log interfaces.Logger) (*Bot, error) {
	pref := tb.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tb.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}

	return &Bot{
		b:            b,
		log:          log,
		cfg:          cfg,
		Menus:        make(map[string]*CommandMenu),
		dynamicMenus: make(map[string]*ComponentMenu),
		actionMap:    make(map[string]CallbackAction),
		publishers:   make(map[string]tele_interfaces.Publisher),
	}, nil
}

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
