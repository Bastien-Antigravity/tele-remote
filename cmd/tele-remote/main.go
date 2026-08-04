package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/subscribers"

	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/routes"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	unilog "github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	unilog_config "github.com/Bastien-Antigravity/universal-logger/src/config"
)

// TeleRemoteCap matches the specific capability for this service
type TeleRemoteCap struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
	URL    string `json:"url"`
	IP     string `json:"ip"`
	Port   string `json:"port"`
}

func main() {
	// 1. Initialize Configuration via Toolbox (handles --profile automatically)
	appConfig, err := toolbox_config.LoadConfig("standalone", nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger (Standardized Bootstrap)
	_, log := unilog.Init("tele-remote", appConfig.Profile, "no_lock", "INFO", false, &unilog_config.DistConfig{Config: appConfig.Config})
	defer log.Close()

	appConfig.Logger = log
	log.Info("Tele-Remote starting with profile: %s", appConfig.Profile)

	// 3. Extract Capabilities
	var tr TeleRemoteCap
	if err := appConfig.Config.GetCapability("tele_remote", &tr); err != nil {
		log.Critical("Tele-Remote capability missing: %v", err)
		os.Exit(1)
	}

	token := strings.TrimSpace(strings.Trim(tr.Token, "\""))
	chatID := strings.TrimSpace(strings.Trim(tr.ChatID, "\""))
	url := strings.TrimSpace(tr.URL)
	url = strings.TrimRight(url, "/")

	if token == "" {
		log.Critical("Tele-Remote token is missing or empty")
		os.Exit(1)
	}
	if chatID == "" {
		log.Critical("Tele-Remote chat_id is missing or empty")
		os.Exit(1)
	}
	if url == "" {
		log.Critical("Tele-Remote url is missing or empty")
		os.Exit(1)
	}

	bindAddr, err := appConfig.GetListenAddr("tele_remote")
	if err != nil {
		log.Critical("Failed to resolve bind address for tele_remote: %v", err)
		os.Exit(1)
	}
	bindIP, bindPortStr, err := net.SplitHostPort(bindAddr)
	if err != nil {
		log.Critical("Failed to parse bind address '%s': %v", bindAddr, err)
		os.Exit(1)
	}
	var bindPort int
	if _, err := fmt.Sscanf(bindPortStr, "%d", &bindPort); err != nil {
		log.Critical("Invalid bind port '%s': %v", bindPortStr, err)
		os.Exit(1)
	}

	// 4. Initialize Telegram Bot Core
	bot, err := core.NewBot(token, url, chatID, log)
	if err != nil {
		log.Critical("Failed to initialize Bot: %v", err)
		os.Exit(1)
	}

	// 5. Setup Routes
	routes.SetupRoutes(bot)

	// 6. Wrap Bot methods into Subscriber Callbacks
	botCallbacks := tele_interfaces.ISubscriberCallbacks{
		OnTelemetry: bot.OnTelemetry,
		OnRegistration: func(clientID, componentName, menuJSON string, pub tele_interfaces.IPublisher) {
			ui.OnComponentConnected(bot, clientID, componentName, menuJSON, pub)
		},
		OnDisconnect: bot.OnDisconnect,
	}

	// 7. Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 8. Start Subscribers (Transport Layer)
	grpcSub := subscribers.NewGrpcSubscriber(log, bindIP, bindPort)
	go func() {
		if err := grpcSub.StartListen(ctx, botCallbacks); err != nil {
			log.Error("gRPC Subscriber failed: %v", err)
		}
	}()
	lm.Register("gRPC_Subscriber", grpcSub.Close)

	// 9. Start the Bot
	go bot.Start(ctx)

	// 10. Wait for Shutdown Signals
	log.Info("Service is ready and listening for commands")
	lm.Wait(ctx)

	log.Info("Tele-Remote shutdown complete")
}
