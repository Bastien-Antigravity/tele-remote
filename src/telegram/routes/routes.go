package routes

/*
ESSENTIAL PROCESS:
Registers global Telebot middlewares (LoggingMiddleware and AuthMiddleware) and
wires top-level Telegram command and reply-keyboard handlers.

DATA FLOW:
1. SetupRoutes attaches LoggingMiddleware and AuthMiddleware to the bot instance prior to route registration.
2. /start resets the caller's scoped ConversationKey state and renders the main menu with the start banner.
3. Critical emergency command ("🆘 power off !") initiates a 2-step confirmation workflow rather than broadcasting immediately.
4. Dynamic reply-keyboard text and confirmation responses are routed through ui.HandleDynamicText.

KEY PARAMETERS:
- bot: Initialized core.Bot instance holding state maps and Telebot router.
- startImage: Embedded PNG banner sent on /start.
*/

import (
	"bytes"
	_ "embed"
	"sync"

	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

//go:embed assets/start.png
var startImage []byte

var (
	cachedStartFileID string
	cachedFileIDMu    sync.RWMutex
)

// -----------------------------------------------------------------------------

// SetupRoutes initializes global middlewares, static handlers, and dynamic reply-keyboard routing.
func SetupRoutes(bot *core.Bot) {
	bot.B.Use(bot.RateLimitMiddleware(), bot.AuthMiddleware(), bot.LoggingMiddleware())

	m := &tb.ReplyMarkup{}
	btnPowerOff := m.Text(models.BtnPowerOff)

	resetAndShowMainMenu := func(c tb.Context, trigger string) error {
		convKey := core.GetConvKey(c)
		bot.Log.Info("User triggered menu command", "cmd", trigger, "chat_id", core.MaskChatID(convKey.ChatID), "user", core.MaskChatID(convKey.UserID))

		// Reset user state to main menu
		bot.Mu.Lock()
		delete(bot.UserStates, convKey)
		delete(bot.UserPaths, convKey)
		delete(bot.PendingInputs, convKey)
		delete(bot.PendingConfirmations, convKey)
		bot.Mu.Unlock()

		if trigger == "/start" && len(startImage) != 0 {
			cachedFileIDMu.RLock()
			fileID := cachedStartFileID
			cachedFileIDMu.RUnlock()

			if fileID != "" {
				photo := &tb.Photo{File: tb.File{FileID: fileID}}
				_ = bot.Send(c, photo)
			} else {
				photo := &tb.Photo{File: tb.FromReader(bytes.NewReader(startImage))}
				if sentMsg, err := bot.B.Send(c.Recipient(), photo); err == nil && sentMsg != nil && sentMsg.Photo != nil {
					cachedFileIDMu.Lock()
					cachedStartFileID = sentMsg.Photo.FileID
					cachedFileIDMu.Unlock()
					bot.Log.Info("Start banner cached on Telegram CDN for fast-path reuse")
				}
			}
		}

		return ui.ShowMainMenu(bot, c)
	}

	bot.B.Handle("/start", func(c tb.Context) error {
		return resetAndShowMainMenu(c, "/start")
	})

	bot.B.Handle("/menu", func(c tb.Context) error {
		return resetAndShowMainMenu(c, "/menu")
	})

	bot.B.Handle(&btnPowerOff, func(c tb.Context) error {
		return ui.RequestEmergencyConfirmation(bot, c, models.CmdPowerOff)
	})

	// Global Handler for dynamic Reply buttons
	bot.B.Handle(tb.OnText, func(c tb.Context) error {
		return ui.HandleDynamicText(bot, c)
	})

	bot.B.Handle(tb.OnCallback, func(c tb.Context) error {
		return c.Respond()
	})
}
