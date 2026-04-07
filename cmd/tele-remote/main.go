package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/config"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/subscribers"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram"

	"github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	"github.com/Bastien-Antigravity/universal-logger/src/utils"
)

// -----------------------------------------------------------------------------
// main is the entry point orchestrating configurations, gRPC, and Telegram bots
func main() {
	// 1. Initialize Settings
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	var loggerProfile string
	var logLevel utils.Level
	if cfg.LogLevel == "DEBUG" {
		loggerProfile = "devel"
		logLevel = utils.LevelDebug
	} else {
		loggerProfile = "minimal"
		logLevel = utils.LevelInfo
	}

	_, appLogger := bootstrap.Init("TeleRemote", "standalone", loggerProfile, logLevel, false)

	appLogger.Info("Bootstrapping TeleRemote Service...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Setup Telegram Bot early to use it as a callback telemetry for gRPC
	// (Deferred instantiation using a channel/func proxy to prevent circular dep)
	var telemetryCallback func(string)
	var botInstance *telegram.Bot

	cbs := interfaces.SubscriberCallbacks{
		OnTelemetry: func(msg string) {
			if telemetryCallback != nil {
				telemetryCallback(msg)
			}
		},
		OnRegistration: func(clientID, componentName, menuJSON string, pub interfaces.Publisher) {
			if botInstance != nil {
				botInstance.OnComponentConnected(clientID, componentName, menuJSON, pub)
			}
		},
		OnDisconnect: func(clientID string) {
			if botInstance != nil {
				botInstance.OnComponentDisconnected(clientID)
			}
		},
	}

	grpcSub := subscribers.NewGrpcSubscriber(appLogger, cfg.BindIP, cfg.BindPort)
	natsSub := subscribers.NewNatsSubscriber(cfg, appLogger)
	safeSub := subscribers.NewSafeSocketSubscriber(cfg, appLogger)

	bot, err := telegram.NewBot(cfg, appLogger)
	botInstance = bot
	if err != nil {
		appLogger.Error("Failed to init telegram bot", "err", err)
		os.Exit(1)
	}

	telemetryCallback = func(msg string) {
		bot.Broadcast(msg)
	}

	// 4. Start concurrent services
	go func() {
		if err := grpcSub.StartListen(ctx, cbs); err != nil {
			appLogger.Error("gRPC server crashed", "err", err)
			cancel()
		}
	}()

	go func() {
		if err := natsSub.StartListen(ctx, cbs); err != nil {
			appLogger.Error("NATS server crashed", "err", err)
		}
	}()

	go func() {
		if err := safeSub.StartListen(ctx, cbs); err != nil {
			appLogger.Error("SafeSocket server crashed", "err", err)
		}
	}()

	go func() {
		bot.Start(ctx)
	}()

	// 5. Block until graceful shutdown via signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	s := <-sigCh
	appLogger.Info("Received signal, initiating graceful shutdown", "signal", s)
	cancel()

	// Wait a moment for grpc and bot components to cleanly stop
	time.Sleep(2 * time.Second)
	appLogger.Info("Shutdown complete.")
}
