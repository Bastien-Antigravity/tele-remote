package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tele-remote/src/config"

	"tele-remote/src/grpc_control"
	"github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------
// Bot holds the telegram connection, config, and state references
type Bot struct {
	b       *tb.Bot
	log     interfaces.Logger
	cfg     *config.Config
	grpcSrv *grpc_control.Server

	Menus map[string]*CommandMenu

	mu           sync.RWMutex
	dynamicMenus map[string]*ComponentMenu
	actionMap    map[string]CallbackAction
	cbCounter    int
}

// -----------------------------------------------------------------------------
// NewBot registers Telebot settings and initializes memory maps
func NewBot(cfg *config.Config, log interfaces.Logger, grpcSrv *grpc_control.Server) (*Bot, error) {
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
		grpcSrv:      grpcSrv,
		Menus:        make(map[string]*CommandMenu),
		dynamicMenus: make(map[string]*ComponentMenu),
		actionMap:    make(map[string]CallbackAction),
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
	chat, err := bot.b.ChatByID(0)

	var chatID int64
	fmt.Sscanf(bot.cfg.ChatID, "%d", &chatID)

	chat = &tb.Chat{ID: chatID}

	_, err = bot.b.Send(chat, msg)
	if err != nil {
		bot.log.Error("failed to broadcast telemetry", "err", err)
	}
}
