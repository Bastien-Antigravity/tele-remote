package routes

import (
	"bytes"
	"context"
	_ "embed"

	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	tb "gopkg.in/telebot.v3"
)

//go:embed assets/start.png
var startImage []byte

// SetupRoutes initializes all static handlers and maps fallback dynamic routing
func SetupRoutes(bot *core.Bot) {
	m := &tb.ReplyMarkup{}
	btnPowerOff := m.Text("🆘 power off !")
	btnCloseAll := m.Text("⏏️ close all positions")

	bot.B.Handle("/start", func(c tb.Context) error {
		bot.Log.Info("User triggered /start", "user", c.Sender().ID)
		
		// Reset user state to main menu
		bot.Mu.Lock()
		delete(bot.UserStates, c.Chat().ID)
		delete(bot.UserPaths, c.Chat().ID)
		bot.Mu.Unlock()

		if len(startImage) != 0 {
			photo := &tb.Photo{File: tb.FromReader(bytes.NewReader(startImage))}
			_ = bot.Send(c, photo)
		}
		
		return ui.ShowMainMenu(bot, c)
	})

	bot.B.Handle(&btnPowerOff, func(c tb.Context) error {
		bot.Log.Info("PowerOff triggered via Telegram")
		
		bot.Mu.RLock()
		for _, pub := range bot.Publishers {
			_ = pub.PublishCommand(context.Background(), int32(models.CmdPowerOff), "", "") 
		}
		bot.Mu.RUnlock()

		return bot.Send(c, "🆘 Powering off all components...")
	})

	bot.B.Handle(&btnCloseAll, func(c tb.Context) error {
		bot.Log.Info("CloseAllPositions triggered via Telegram")
		
		bot.Mu.RLock()
		for _, pub := range bot.Publishers {
			_ = pub.PublishCommand(context.Background(), int32(models.CmdStop), "", "")
		}
		bot.Mu.RUnlock()

		return bot.Send(c, "🛑 Calling 'stop' on all components...")
	})

	// Global Handler for dynamic Reply buttons
	bot.B.Handle(tb.OnText, func(c tb.Context) error {
		return ui.HandleDynamicText(bot, c)
	})

	bot.B.Handle(tb.OnCallback, func(c tb.Context) error {
		return c.Respond()
	})
}
