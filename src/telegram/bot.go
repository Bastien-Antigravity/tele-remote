package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/Bastien-Antigravity/TeleRemote/internal/config"
	"github.com/Bastien-Antigravity/TeleRemote/internal/grpc"
	"github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	tb "gopkg.in/telebot.v3"
)

type Bot struct {
	b       *tb.Bot
	log     interfaces.Logger
	cfg     *config.Config
	grpcSrv *grpc.Server

	// Handlers structured map
	Menus map[string]*CommandMenu
}

func NewBot(cfg *config.Config, log interfaces.Logger, grpcSrv *grpc.Server) (*Bot, error) {
	pref := tb.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tb.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}

	return &Bot{
		b:       b,
		log:     log,
		cfg:     cfg,
		grpcSrv: grpcSrv,
		Menus:   make(map[string]*CommandMenu),
	}, nil
}

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

// Broadcast sends a message to the configured ChatID
func (bot *Bot) Broadcast(msg string) {
	if bot.cfg.ChatID == "" {
		bot.log.Warning("Broadcast failed: TB_CHATID not set")
		return
	}
	chat, err := bot.b.ChatByID(0) // Need explicit chatID parsing, handle string logic

	// Temporary parsing logic to integer
	var chatID int64
	fmt.Sscanf(bot.cfg.ChatID, "%d", &chatID)

	chat = &tb.Chat{ID: chatID}

	_, err = bot.b.Send(chat, msg)
	if err != nil {
		bot.log.Error("failed to broadcast telemetry", "err", err)
	}
}

func (bot *Bot) setupRoutes() {
	// Constructing the Main Menu using Command Pattern interfaces inline for brevity
	menuStart := &tb.ReplyMarkup{}

	btnPowerOff := menuStart.Data("🆘 power off !", "power_off")
	btnCloseAll := menuStart.Data("⏏️ close all positions", "close_all")
	btnStrategies := menuStart.Data("🍀 running strategies", "strategies")
	btnArbitrage := menuStart.Data("📈📉 arbitrage", "arbitrage")

	menuStart.Inline(
		menuStart.Row(btnPowerOff),
		menuStart.Row(btnCloseAll),
		menuStart.Row(btnStrategies),
		menuStart.Row(btnArbitrage),
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
}

// Concept for structured command menus if we wanted to build nested structures later
type CommandMenu struct {
	Name    string
	Caption string
	Markup  *tb.ReplyMarkup
}
