package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bastien-Antigravity/tele-remote/src/config"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/store"
	"github.com/Bastien-Antigravity/tele-remote/src/subscribers"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram"

	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	unilog "github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	unilog_config "github.com/Bastien-Antigravity/universal-logger/src/config"
)

// -----------------------------------------------------------------------------
// Main Entry Point
// -----------------------------------------------------------------------------

func main() {
	// 1. Initialize Configuration via Toolbox
	profile := os.Getenv("TR_PROFILE")
	if profile == "" {
		profile = "standalone"
	}

	cfg, err := config.LoadConfig(profile)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------------

	// 2. Initialize Logger (Standardized Bootstrap)
	_, log := unilog.Init("tele-remote", profile, "no_lock", "INFO", false, &unilog_config.DistConfig{Config: cfg.Config})
	defer log.Close()

	// Inject logger into Config for toolbox internal logs
	cfg.AppConfig.Logger = log
	log.Info("Tele-Remote starting with profile: %s", profile)

	// -----------------------------------------------------------------------------

	// 3. Initialize Persistence (Minimal State)
	pm := store.NewPersistenceManager("src/assets/registry_state.json", log)

	// 4. Initialize Telegram Bot
	bot, err := telegram.NewBot(cfg, log, pm)
	if err != nil {
		log.Critical("Failed to initialize Bot: %v", err)
		os.Exit(1)
	}

	// 5. Wrap Bot methods into Subscriber Callbacks
	botCallbacks := tele_interfaces.SubscriberCallbacks{
		OnTelemetry:    bot.OnTelemetry,
		OnRegistration: bot.OnComponentConnected,
		OnDisconnect:   bot.OnDisconnect,
	}

	// -----------------------------------------------------------------------------

	// 6. Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(log)

	// Register final state flush on shutdown
	lm.Register("PersistenceFlush", bot.SaveState)

	// Context for background listeners
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// -----------------------------------------------------------------------------

	// 7. Start Subscribers (Transport Layer)
	
	// NATS Subscriber
	natsSub := subscribers.NewNatsSubscriber(cfg, log)
	if err := natsSub.StartListen(ctx, botCallbacks); err != nil {
		log.Error("NATS Subscriber failed to start: %v", err)
	}
	lm.Register("NATS_Subscriber", natsSub.Close)

	// gRPC Subscriber
	grpcSub := subscribers.NewGrpcSubscriber(log, cfg.BindIP, cfg.BindPort)
	go func() {
		if err := grpcSub.StartListen(ctx, botCallbacks); err != nil {
			log.Error("gRPC Subscriber failed: %v", err)
		}
	}()
	lm.Register("gRPC_Subscriber", grpcSub.Close)

	// SafeSocket Subscriber (Future)
	ssSub := subscribers.NewSafeSocketSubscriber(cfg, log)
	if err := ssSub.StartListen(ctx, botCallbacks); err != nil {
		log.Error("SafeSocket Subscriber failed: %v", err)
	}
	lm.Register("SafeSocket_Subscriber", ssSub.Close)

	// -----------------------------------------------------------------------------

	// 8. Start the Bot
	go bot.Start(ctx)

	// 9. Wait for Shutdown Signals via Toolbox Lifecycle
	log.Info("Service is ready and listening for commands")
	lm.Wait(ctx)
	
	log.Info("Tele-Remote shutdown complete")
}
