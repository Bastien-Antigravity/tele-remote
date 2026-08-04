package core

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
	tb "gopkg.in/telebot.v3"
)

// Config is a simple alias for the toolbox AppConfig in this context
type Config struct {
	TelegramToken string
	TelegramURL   string
	ChatID        string
}

// Bot holds the telegram connection, config, and state references
type Bot struct {
	B   *tb.Bot
	Log unilog_ifaces.Logger
	Cfg *Config

	Mu            sync.RWMutex
	DynamicMenus  map[string]*models.ComponentMenu
	ActionMap     map[string]models.CallbackAction
	CbCounter     int
	Publishers    map[string]tele_interfaces.IPublisher
	UserStates    map[int64]string                // chatID -> current component ID
	PendingInputs map[int64]*models.CommandButton // chatID -> button waiting for text
	UserPaths     map[int64][]string              // chatID -> breadcrumbs for "Back" button
}

// NewBot registers Telebot settings and initializes memory maps
func NewBot(token, url, chatID string, log unilog_ifaces.Logger) (*Bot, error) {
	pref := tb.Settings{
		Token:  token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	if url != "" {
		pref.URL = url
	} else {
		pref.URL = "https://api.telegram.org"
	}

	b, err := tb.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}

	cfg := &Config{
		TelegramToken: token,
		TelegramURL:   pref.URL,
		ChatID:        chatID,
	}

	bot := &Bot{
		B:             b,
		Log:           log,
		Cfg:           cfg,
		DynamicMenus:  make(map[string]*models.ComponentMenu),
		ActionMap:     make(map[string]models.CallbackAction),
		Publishers:    make(map[string]tele_interfaces.IPublisher),
		UserStates:    make(map[int64]string),
		PendingInputs: make(map[int64]*models.CommandButton),
		UserPaths:     make(map[int64][]string),
	}

	return bot, nil
}

// Start begins polling
func (bot *Bot) Start(ctx context.Context) {
	bot.B.Use(bot.LoggingMiddleware())

	go func() {
		<-ctx.Done()
		bot.Log.Info("Shutting down Telebot gracefully...")
		bot.B.Stop()
	}()

	bot.Log.Info("Telebot starting polling...", "chatID", bot.Cfg.ChatID)
	bot.B.Start()
}

// Broadcast sends a plain text message to the pre-configured ChatID
func (bot *Bot) Broadcast(msg string) {
	if bot.Cfg.ChatID == "" {
		bot.Log.Warning("Broadcast failed: TB_CHATID not set")
		return
	}

	var chatID int64
	var err error
	if parsedID, parseErr := strconv.ParseInt(bot.Cfg.ChatID, 10, 64); parseErr == nil {
		chatID = parsedID
	} else {
		bot.Log.Error("Broadcast failed: invalid ChatID format (must be numeric)", "chat_id", bot.Cfg.ChatID, "err", parseErr)
		return
	}

	chat := &tb.Chat{ID: chatID}

	_, err = bot.B.Send(chat, msg)
	if err != nil {
		bot.Log.Error("failed to broadcast telemetry", "err", err)
	}
}

// OnTelemetry handles incoming logs or events by broadcasting them to the admin chat
func (bot *Bot) OnTelemetry(msg string) {
	bot.Broadcast(msg)
}

// OnDisconnect cleans up a component's state when it loses connection
func (bot *Bot) OnDisconnect(clientID string) {
	bot.Mu.Lock()
	defer bot.Mu.Unlock()

	if _, ok := bot.DynamicMenus[clientID]; ok {
		bot.Log.Info("Removing session for disconnected component", "client", clientID)
		delete(bot.DynamicMenus, clientID)
		delete(bot.Publishers, clientID)
	}
}

// LoggingMiddleware intercepts all incoming messages for auditing
func (bot *Bot) LoggingMiddleware() tb.MiddlewareFunc {
	return func(next tb.HandlerFunc) tb.HandlerFunc {
		return func(c tb.Context) error {
			if c.Message() != nil {
				bot.Log.Info("Received message", "chat_id", c.Chat().ID, "user", c.Sender().ID, "text", c.Text())
			} else if c.Callback() != nil {
				bot.Log.Info("Received callback", "chat_id", c.Chat().ID, "user", c.Sender().ID, "data", c.Callback().Data)
			}
			return next(c)
		}
	}
}

// Send wraps tb.Context.Send to intercept and log all outgoing responses
func (bot *Bot) Send(c tb.Context, what interface{}, opts ...interface{}) error {
	bot.Log.Info("Sending response", "chat_id", c.Chat().ID, "content", fmt.Sprintf("%v", what))
	return c.Send(what, opts...)
}
