package core

/*
ESSENTIAL PROCESS:
Manages the Telegram Bot lifecycle, ChatID authorization middleware, non-blocking
telemetry broadcast worker, and session-safe in-memory state registry for connected microservices.

DATA FLOW:
1. NewBot initializes the Telebot instance, state maps, and background telemetry worker.
2. AuthMiddleware validates every incoming Telegram update against the configured ChatID before handler execution.
3. OnTelemetry enqueues incoming component messages into a bounded channel (cap 256) for asynchronous Telegram delivery.
4. OnDisconnect verifies publisher identity before removing component sessions and purges stale user navigation/input state.

KEY PARAMETERS:
- Cfg.ChatID: Authorized Telegram chat/user ID permitted to interact with the bot.
- telemetryChan: Bounded 256-slot channel decoupling gRPC stream reads from Telegram HTTP calls.
- UserStates / UserPaths / PendingInputs / PendingConfirmations: Per-ConversationKey interactive state maps.
*/

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

// ConversationKey aliases models.ConversationKey for convenient access across packages.
type ConversationKey = models.ConversationKey

// PendingInput aliases models.PendingInput for convenient access across packages.
type PendingInput = models.PendingInput

// PendingConfirmation aliases models.PendingConfirmation for convenient access across packages.
type PendingConfirmation = models.PendingConfirmation

// -----------------------------------------------------------------------------

// GetConvKey builds a scoped ConversationKey{ChatID, UserID} from a Telebot context,
// falling back to UserID: 0 when c.Sender() is nil.
func GetConvKey(c tb.Context) ConversationKey {
	if c == nil {
		return ConversationKey{}
	}
	var chatID, userID int64
	if c.Chat() != nil {
		chatID = c.Chat().ID
	}
	if c.Sender() != nil {
		userID = c.Sender().ID
	}
	return ConversationKey{
		ChatID: chatID,
		UserID: userID,
	}
}

// -----------------------------------------------------------------------------

// Config holds the Telegram bot connection and authorization settings.
type Config struct {
	TelegramToken string
	TelegramURL   string
	ChatID        string
	AllowedUsers  string
	AuthorizedIDs map[int64]struct{}
	BroadcastIDs  []int64
}

// -----------------------------------------------------------------------------

// parseAuthorizedIDs extracts unique numeric IDs from chatID and allowedUsers inputs on the cold path.
func parseAuthorizedIDs(chatID string, allowedUsers ...string) (map[int64]struct{}, []int64) {
	idSet := make(map[int64]struct{})
	var broadcastList []int64

	allSources := append([]string{chatID}, allowedUsers...)
	for _, src := range allSources {
		tokens := strings.FieldsFunc(src, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
		})
		for _, tok := range tokens {
			trimmed := strings.TrimSpace(tok)
			if trimmed == "" {
				continue
			}
			if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				if _, exists := idSet[id]; !exists {
					idSet[id] = struct{}{}
					broadcastList = append(broadcastList, id)
				}
			}
		}
	}
	return idSet, broadcastList
}

// -----------------------------------------------------------------------------

// Bot holds the Telegram connection, config, non-blocking telemetry queue, and live state registry.
type Bot struct {
	B       *tb.Bot
	Log     unilog_ifaces.Logger
	Cfg     *Config
	Limiter *RateLimiter

	Mu                   sync.RWMutex
	DynamicMenus         map[string]*models.ComponentMenu
	Publishers           map[string]tele_interfaces.IPublisher
	UserStates           map[ConversationKey]string
	PendingInputs        map[ConversationKey]PendingInput
	PendingConfirmations map[ConversationKey]PendingConfirmation
	UserPaths            map[ConversationKey][]string

	telemetryChan chan string
	stopTelemetry chan struct{}
	stopOnce      sync.Once
}

// -----------------------------------------------------------------------------

// NewBot registers Telebot settings, initializes memory maps, and starts the background telemetry worker.
func NewBot(token, url, chatID string, log unilog_ifaces.Logger, allowedUsers ...string) (*Bot, error) {
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

	allowedStr := ""
	if len(allowedUsers) > 0 {
		allowedStr = strings.Join(allowedUsers, ",")
	}

	authIDs, broadcastIDs := parseAuthorizedIDs(chatID, allowedUsers...)

	cfg := &Config{
		TelegramToken: token,
		TelegramURL:   pref.URL,
		ChatID:        chatID,
		AllowedUsers:  allowedStr,
		AuthorizedIDs: authIDs,
		BroadcastIDs:  broadcastIDs,
	}

	bot := &Bot{
		B:                    b,
		Log:                  log,
		Cfg:                  cfg,
		Limiter:              NewRateLimiter(DefaultBurstLimit, DefaultRateWindow, DefaultRateCooldown),
		DynamicMenus:         make(map[string]*models.ComponentMenu),
		Publishers:           make(map[string]tele_interfaces.IPublisher),
		UserStates:           make(map[ConversationKey]string),
		PendingInputs:        make(map[ConversationKey]PendingInput),
		PendingConfirmations: make(map[ConversationKey]PendingConfirmation),
		UserPaths:            make(map[ConversationKey][]string),
		telemetryChan:        make(chan string, 256),
		stopTelemetry:        make(chan struct{}),
	}

	go bot.runTelemetryWorker()

	return bot, nil
}

// -----------------------------------------------------------------------------

func (bot *Bot) runTelemetryWorker() {
	for {
		select {
		case msg, ok := <-bot.telemetryChan:
			if !ok {
				return
			}
			bot.Broadcast(msg)
		case <-bot.stopTelemetry:
			for {
				select {
				case msg := <-bot.telemetryChan:
					bot.Broadcast(msg)
				default:
					return
				}
			}
		}
	}
}

// -----------------------------------------------------------------------------

// Start begins polling and shuts down cleanly when ctx is cancelled.
func (bot *Bot) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		bot.Log.Info("Shutting down Telebot gracefully...")
		bot.stopOnce.Do(func() {
			close(bot.stopTelemetry)
		})
		bot.B.Stop()
	}()

	bot.Log.Info("Telebot starting polling...", "chatID", bot.Cfg.ChatID)
	bot.B.Start()
}

// -----------------------------------------------------------------------------

const maxTelegramMessageLen = 4000

// splitTelegramMessage splits a message into chunks of at most maxLen runes,
// preferring newline boundaries in the second half of each window.
func splitTelegramMessage(msg string, maxLen int) []string {
	if msg == "" || maxLen <= 0 {
		return []string{msg}
	}

	runes := []rune(msg)
	if len(runes) <= maxLen {
		return []string{msg}
	}

	var chunks []string
	for len(runes) > maxLen {
		splitAt := maxLen
		for i := maxLen - 1; i > maxLen/2; i-- {
			if runes[i] == '\n' {
				splitAt = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:splitAt]))
		runes = runes[splitAt:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

// -----------------------------------------------------------------------------

// IsAuthorized checks whether the incoming Telegram sender (or private chat when sender is nil)
// matches any configured ChatID or AllowedUsers.
func (bot *Bot) IsAuthorized(c tb.Context) bool {
	if bot == nil || bot.Cfg == nil || len(bot.Cfg.AuthorizedIDs) == 0 || c == nil {
		return false
	}
	if c.Sender() != nil {
		_, ok := bot.Cfg.AuthorizedIDs[c.Sender().ID]
		return ok
	}
	if c.Chat() != nil && c.Chat().ID > 0 {
		_, ok := bot.Cfg.AuthorizedIDs[c.Chat().ID]
		return ok
	}
	return false
}

// -----------------------------------------------------------------------------

// SetBurstLimit dynamically updates the RateLimiter's maximum burst threshold.
func (bot *Bot) SetBurstLimit(limit int) {
	if bot != nil && limit > 0 {
		bot.Limiter = NewRateLimiter(limit, DefaultRateWindow, DefaultRateCooldown)
	}
}

// -----------------------------------------------------------------------------

// AuthMiddleware rejects any Telegram update from an unauthorized ChatID/Sender.
func (bot *Bot) AuthMiddleware() tb.MiddlewareFunc {
	return func(next tb.HandlerFunc) tb.HandlerFunc {
		return func(c tb.Context) error {
			if !bot.IsAuthorized(c) {
				var chatID, userID int64
				var username string
				if c != nil && c.Chat() != nil {
					chatID = c.Chat().ID
				}
				if c != nil && c.Sender() != nil {
					userID = c.Sender().ID
					username = c.Sender().Username
				}
				bot.Log.Warning("Unauthorized Telegram access attempt rejected",
					"chat_id", chatID, "user_id", userID, "username", username)
				return bot.Send(c, "⛔ Unauthorized access.")
			}
			return next(c)
		}
	}
}

// -----------------------------------------------------------------------------

// Broadcast sends a plain text message to all pre-configured ChatIDs, chunking long payloads.
func (bot *Bot) Broadcast(msg string) {
	if bot.Cfg == nil || len(bot.Cfg.BroadcastIDs) == 0 {
		bot.Log.Warning("Broadcast failed: no authorized broadcast chat IDs configured")
		return
	}

	chunks := splitTelegramMessage(msg, maxTelegramMessageLen)
	for _, chatID := range bot.Cfg.BroadcastIDs {
		chat := &tb.Chat{ID: chatID}
		for _, chunk := range chunks {
			if _, err := bot.B.Send(chat, chunk); err != nil {
				bot.Log.Error("Failed to broadcast telemetry to chat", "chat_id", chatID, "err", err)
			}
		}
	}
}

// -----------------------------------------------------------------------------

// OnTelemetry enqueues incoming logs or events non-blockingly for asynchronous delivery to the admin chat.
func (bot *Bot) OnTelemetry(msg string) {
	select {
	case bot.telemetryChan <- msg:
	default:
		bot.Log.Warning("Telemetry queue full; dropping message")
	}
}

// -----------------------------------------------------------------------------

// OnDisconnect cleans up a component's state when it loses connection.
// If pub is provided, the session is only removed if pub still matches the active publisher for clientID,
// preventing a late disconnect from an older stream from deleting a newly reconnected session.
func (bot *Bot) OnDisconnect(clientID string, pub ...tele_interfaces.IPublisher) {
	bot.Mu.Lock()
	defer bot.Mu.Unlock()

	if len(pub) > 0 && pub[0] != nil {
		activePub, exists := bot.Publishers[clientID]
		if !exists || activePub != pub[0] {
			bot.Log.Info("Ignoring stale disconnect for superseded component session", "client", clientID)
			return
		}
	}

	_, hasMenu := bot.DynamicMenus[clientID]
	_, hasPub := bot.Publishers[clientID]
	if hasMenu || hasPub {
		bot.Log.Info("Removing session for disconnected component", "client", clientID)
		delete(bot.DynamicMenus, clientID)
		delete(bot.Publishers, clientID)
	}

	// Clean up any UserStates, UserPaths, and PendingInputs pointing to the disconnected clientID
	for key, stateClientID := range bot.UserStates {
		if stateClientID == clientID {
			delete(bot.UserStates, key)
			delete(bot.UserPaths, key)
		}
	}
	for key, pending := range bot.PendingInputs {
		if pending.ClientID == clientID {
			delete(bot.PendingInputs, key)
		}
	}
}

// -----------------------------------------------------------------------------

// LoggingMiddleware intercepts all incoming messages for auditing.
func (bot *Bot) LoggingMiddleware() tb.MiddlewareFunc {
	return func(next tb.HandlerFunc) tb.HandlerFunc {
		return func(c tb.Context) error {
			var chatID, userID int64
			if c != nil && c.Chat() != nil {
				chatID = c.Chat().ID
			}
			if c != nil && c.Sender() != nil {
				userID = c.Sender().ID
			}
			if c != nil && c.Message() != nil {
				convKey := GetConvKey(c)
				bot.Mu.RLock()
				_, hasPending := bot.PendingInputs[convKey]
				bot.Mu.RUnlock()

				loggedText := c.Text()
				if hasPending {
					loggedText = "[REDACTED_PENDING_INPUT]"
				}
				bot.Log.Info("Received message", "chat_id", chatID, "user", userID, "text", loggedText)
			} else if c != nil && c.Callback() != nil {
				bot.Log.Info("Received callback", "chat_id", chatID, "user", userID, "data", c.Callback().Data)
			}
			return next(c)
		}
	}
}

// -----------------------------------------------------------------------------

// Send wraps tb.Context.Send to intercept and log all outgoing responses.
func (bot *Bot) Send(c tb.Context, what interface{}, opts ...interface{}) error {
	var chatID int64
	if c != nil && c.Chat() != nil {
		chatID = c.Chat().ID
	}
	bot.Log.Info("Sending response", "chat_id", chatID, "content", fmt.Sprintf("%v", what))
	return c.Send(what, opts...)
}
