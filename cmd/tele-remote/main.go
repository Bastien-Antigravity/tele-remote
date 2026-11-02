package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	toolbox_bootstrap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/bootstrap"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/subscribers"

	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/routes"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
	toolbox_version "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/version"
)

// TeleRemoteCap matches the specific capability for this service
type TeleRemoteCap struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
	URL    string `json:"url"`
	IP     string `json:"ip"`
	Port   string `json:"port"`
}

// ServerVersion defines the release version reported in logs and telemetry.
// Dynamically resolved from microservice-toolbox (BuildVersion link-time flag or VERSION.txt).
var ServerVersion = toolbox_version.Get()

func main() {
	// 1. Initialize Service via Unified Ecosystem Bootstrapper
	appConfig, log := toolbox_bootstrap.BootstrapService("tele-remote")
	if appConfig == nil {
		panic("tele-remote: bootstrap failed to produce non-nil appConfig")
	}
	if log == nil {
		panic("tele-remote: bootstrap failed to produce non-nil logger")
	}
	defer log.Close()

	log.Info("Tele-Remote (version %s) starting with profile: %s", ServerVersion, appConfig.Profile)

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

	// Sovereign on-demand decryption for tele-remote credentials
	decToken, err := appConfig.DecryptSecret(token)
	if err != nil {
		log.Critical("Failed to decrypt Tele-Remote token: %v", err)
		os.Exit(1)
	}
	token = decToken

	decChatID, err := appConfig.DecryptSecret(chatID)
	if err != nil {
		log.Critical("Failed to decrypt Tele-Remote chat_id: %v", err)
		os.Exit(1)
	}
	chatID = decChatID

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

	grpcAddr, err := appConfig.GetListenAddr("tele_remote")
	if err != nil {
		log.Critical("Failed to resolve bind address for tele_remote: %v", err)
		os.Exit(1)
	}
	bindIP, bindPortStr, err := net.SplitHostPort(grpcAddr)
	if err != nil {
		log.Critical("Failed to parse bind address '%s': %v", grpcAddr, err)
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

	// 9. Register tele-remote specification with web-interface dynamically (linked to lifecycle)
	_ = toolbox_spec.PublishWithLifecycle(appConfig, log, lm, "tele_remote", toolbox_spec.ServiceSpec{
		Name:        "Tele Remote",
		Tag:         "tele-remote",
		URL:         fmt.Sprintf("http://%s", grpcAddr),
		NavTitle:    "Tele Remote",
		Layer:       "L1",
		Category:    "Base Fleet",
		Priority:    40,
		Icon:        "fa-paper-plane",
		Description: "Bidirectional Telegram bridge connecting distributed microservices and worker daemons to interactive bot menus.",
		Protocol:    "gRPC Stream",
		Framing:     fmt.Sprintf("Port %d gRPC Stream", bindPort),
		DocLink:     "obsidian-brain/06-Microservices/Tele-Remote-Hub.md",
	})

	// 10. Start the Bot
	go bot.Start(ctx)

	// 11. Wait for Shutdown Signals
	log.Info("Service is ready and listening for commands")
	lm.Wait(ctx)

	log.Info("Tele-Remote shutdown complete")
}
