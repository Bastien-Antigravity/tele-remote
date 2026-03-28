package telegram

import tb "gopkg.in/telebot.v3"

// -----------------------------------------------------------------------------
// setupRoutes initializes all static handlers and maps fallback dynamic routing
func (bot *Bot) setupRoutes() {
	// Constructing the Main Menu using Command Pattern interfaces inline for brevity
	menuStart := &tb.ReplyMarkup{}

	btnPowerOff := menuStart.Data("🆘 power off !", "power_off")
	btnCloseAll := menuStart.Data("⏏️ close all positions", "close_all")
	btnStrategies := menuStart.Data("🍀 running strategies", "strategies")
	btnArbitrage := menuStart.Data("📈📉 arbitrage", "arbitrage")
	btnNodes := menuStart.Data("🔌 Connected Nodes", "connected_nodes")

	menuStart.Inline(
		menuStart.Row(btnPowerOff),
		menuStart.Row(btnCloseAll),
		menuStart.Row(btnStrategies),
		menuStart.Row(btnArbitrage),
		menuStart.Row(btnNodes),
	)

	bot.b.Handle("/start", func(c tb.Context) error {
		bot.log.Info("User triggered /start", "user", c.Sender().ID)
		return c.Send("start!", menuStart)
	})

	bot.b.Handle(&btnPowerOff, func(c tb.Context) error {
		bot.log.Info("PowerOff triggered via Telegram")
		bot.grpcSrv.PowerOffAll()
		c.Send("🆘 Powering off components...")
		return c.Respond()
	})

	bot.b.Handle(&btnCloseAll, func(c tb.Context) error {
		bot.log.Info("CloseAllPositions triggered via Telegram")
		bot.grpcSrv.StopAllComponents()
		c.Send("🛑 Calling 'stop' on all components...")
		return c.Respond()
	})

	// Fallback logic representing UNDER_CONSTRUCTION
	bot.b.Handle(&btnStrategies, func(c tb.Context) error {
		return c.Send("🚧 under construction ...")
	})
	bot.b.Handle(&btnArbitrage, func(c tb.Context) error {
		return c.Send("🚧 under construction ...")
	})

	bot.b.Handle(&btnNodes, func(c tb.Context) error {
		return bot.showNodesMenu(c)
	})

	// Fallback for dynamic callbacks
	bot.b.Handle(tb.OnCallback, func(c tb.Context) error {
		return bot.handleDynamicCallback(c)
	})
}
