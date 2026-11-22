package routes

/*
ESSENTIAL PROCESS:
Registers global Telebot middlewares (LoggingMiddleware and AuthMiddleware) and
wires top-level Telegram command and reply-keyboard handlers.

DATA FLOW:
1. SetupRoutes attaches LoggingMiddleware and AuthMiddleware to the bot instance prior to route registration.
2. /start resets the caller's scoped ConversationKey state and renders the main menu with the start banner.
3. Critical emergency commands ("🆘 power off !" and "⏏️ close all positions") initiate a 2-step confirmation workflow rather than broadcasting immediately.
4. Dynamic reply-keyboard text and confirmation responses are routed through ui.HandleDynamicText.

KEY PARAMETERS:
- bot: Initialized core.Bot instance holding state maps and Telebot router.
- startImage: Embedded PNG banner sent on /start.
*/

import (
	"bytes"
	_ "embed"

	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

//go:embed assets/start.png
var startImage []byte

// -----------------------------------------------------------------------------

// SetupRoutes initializes global middlewares, static handlers, and dynamic reply-keyboard routing.
func SetupRoutes(bot *core.Bot) {
	bot.B.Use(bot.LoggingMiddleware(), bot.AuthMiddleware())

	m := &tb.ReplyMarkup{}
	btnPowerOff := m.Text(models.BtnPowerOff)
	btnCloseAll := m.Text(models.BtnCloseAll)

	bot.B.Handle("/start", func(c tb.Context) error {
		convKey := core.GetConvKey(c)
		bot.Log.Info("User triggered /start", "chat_id", convKey.ChatID, "user", convKey.UserID)

		// Reset user state to main menu
		bot.Mu.Lock()
		delete(bot.UserStates, convKey)
		delete(bot.UserPaths, convKey)
		delete(bot.PendingInputs, convKey)
		delete(bot.PendingConfirmations, convKey)
		bot.Mu.Unlock()

		if len(startImage) != 0 {
			photo := &tb.Photo{File: tb.FromReader(bytes.NewReader(startImage))}
			_ = bot.Send(c, photo)
		}

		return ui.ShowMainMenu(bot, c)
	})

	bot.B.Handle(&btnPowerOff, func(c tb.Context) error {
		return ui.RequestEmergencyConfirmation(bot, c, models.CmdPowerOff)
	})

	bot.B.Handle(&btnCloseAll, func(c tb.Context) error {
		return ui.RequestEmergencyConfirmation(bot, c, models.CmdStop)
	})

	// Global Handler for dynamic Reply buttons
	bot.B.Handle(tb.OnText, func(c tb.Context) error {
		return ui.HandleDynamicText(bot, c)
	})

	bot.B.Handle(tb.OnCallback, func(c tb.Context) error {
		return c.Respond()
	})
}
