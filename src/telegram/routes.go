package telegram

import (
	"context"

	"github.com/Bastien-Antigravity/tele-remote/src/models"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------
// Routing Setup
// -----------------------------------------------------------------------------

// setupRoutes initializes all static handlers and maps fallback dynamic routing
func (bot *Bot) setupRoutes() {
	// Constructing the Main Menu using Command Pattern interfaces inline for brevity
	menuStart := &tb.ReplyMarkup{}

	btnPowerOff := menuStart.Data("🆘 power off !", "power_off")
	btnCloseAll := menuStart.Data("⏏️ close all positions", "close_all")
	btnNodes := menuStart.Data("🔌 Connected Nodes", "connected_nodes")

	menuStart.Inline(
		menuStart.Row(btnPowerOff),
		menuStart.Row(btnCloseAll),
		menuStart.Row(btnNodes),
	)

	// -----------------------------------------------------------------------------

	bot.b.Handle("/start", func(c tb.Context) error {
		bot.log.Info("User triggered /start", "user", c.Sender().ID)
		// Assets are now located in src/assets
		photo := &tb.Photo{File: tb.FromDisk("src/assets/start.png")}
		return c.Send(photo, menuStart)
	})

	// -----------------------------------------------------------------------------

	bot.b.Handle(&btnPowerOff, func(c tb.Context) error {
		bot.log.Info("PowerOff triggered via Telegram")
		
		bot.mu.RLock()
		for _, pub := range bot.publishers {
			pub.PublishCommand(context.Background(), int32(models.CmdPowerOff), "") 
		}
		bot.mu.RUnlock()

		c.Send("🆘 Powering off components...")
		return c.Respond()
	})

	// -----------------------------------------------------------------------------

	bot.b.Handle(&btnCloseAll, func(c tb.Context) error {
		bot.log.Info("CloseAllPositions triggered via Telegram")
		
		bot.mu.RLock()
		for _, pub := range bot.publishers {
			pub.PublishCommand(context.Background(), int32(models.CmdStop), "")
		}
		bot.mu.RUnlock()

		c.Send("🛑 Calling 'stop' on all components...")
		return c.Respond()
	})

	// -----------------------------------------------------------------------------

	bot.b.Handle(&btnNodes, func(c tb.Context) error {
		return bot.showNodesMenu(c)
	})

	// -----------------------------------------------------------------------------

	bot.b.Handle(tb.OnCallback, func(c tb.Context) error {
		return bot.handleDynamicCallback(c)
	})
}
