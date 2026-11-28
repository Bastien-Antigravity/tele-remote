package main

/*
ESSENTIAL PROCESS:
Entrypoint for the Tele-Remote microservice, bootstrapping config/logging,
decrypting Telegram and gRPC security secrets, synchronously binding the gRPC subscriber
with Option A X.509 TLS/mTLS credentials, launching the Telegram bot, and publishing contracts.
Failure Policy: Fail-Fast with CRITICAL log and os.Exit(1) on missing config, secret decryption failure, or TCP bind error.

DATA FLOW:
1. BootstrapService loads AppConfig and logger, then extracts tele_remote capability and local TLS config.
2. Token, chat_id, allowed_users, and optional auth_token are decrypted via AppConfig.DecryptSecret.
3. GrpcSubscriber is initialized via ConfigureSecurity and Bind (fail-fast cold path) and updated live via OnLiveConfUpdate.
4. Lifecycle manager coordinates graceful shutdown across gRPC transport, Telegram bot worker drain, and spec registration.

KEY PARAMETERS:
- TeleRemoteCap: Shared fleet capability holding Telegram credentials, gRPC bind info, TLS policy, and authToken.
- LocalTLSConfig: Service-private X.509 cert/key/CA paths loaded strictly from local: section.
- appConfig.GetVersion(): Canonical service version resolved strictly from VERSION.txt.
*/

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/subscribers"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/routes"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/ui"

	toolbox_bootstrap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/bootstrap"
	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_network "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/network"
	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
)

// -----------------------------------------------------------------------------

// TeleRemoteCap matches the shared fleet capability for tele-remote.
type TeleRemoteCap struct {
	Token        string      `json:"token"`
	ChatID       string      `json:"chat_id"`
	AllowedUsers string      `json:"allowed_users"`
	URL          string      `json:"url"`
	IP           string      `json:"ip"`
	Port         string      `json:"port"`
	BurstLimit   int         `json:"burst_limit"`
	TLSEnabled   interface{} `json:"tls_enabled"`
	TLSMode      string      `json:"tls_mode"`
	AuthToken    string      `json:"auth_token"`
}

// -----------------------------------------------------------------------------

func parseCapBool(val interface{}) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		trimmed := strings.TrimSpace(v)
		return strings.EqualFold(trimmed, "true") || trimmed == "1"
	default:
		return false
	}
}

// -----------------------------------------------------------------------------

func decryptOptionalSecret(appConfig *toolbox_config.AppConfig, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	return appConfig.DecryptSecret(trimmed)
}

// -----------------------------------------------------------------------------

func buildTLSPolicy(appConfig *toolbox_config.AppConfig, tr TeleRemoteCap) toolbox_network.TLSPolicy {
	return toolbox_network.TLSPolicy{
		Enabled:  parseCapBool(tr.TLSEnabled),
		Mode:     strings.TrimSpace(tr.TLSMode),
		LocalTLS: appConfig.GetLocalTLSConfig("tele_remote"),
	}
}

// -----------------------------------------------------------------------------

func main() {
	// 1. Initialize Service via Unified Ecosystem Bootstrapper
	appConfig, log := toolbox_bootstrap.BootstrapService("tele-remote")
	defer log.Close()

	version := appConfig.GetVersion()
	log.Info("Tele-Remote (version %s) starting with profile: %s", version, appConfig.Profile)

	// 2. Extract Capabilities
	var tr TeleRemoteCap
	if err := appConfig.Config.GetCapability("tele_remote", &tr); err != nil {
		log.Critical("Tele-Remote capability missing: %v", err)
		os.Exit(1)
	}

	token := strings.TrimSpace(tr.Token)
	chatID := strings.TrimSpace(tr.ChatID)
	allowedUsers := strings.TrimSpace(tr.AllowedUsers)
	url := strings.TrimRight(strings.TrimSpace(tr.URL), "/")

	// 3. Decrypt required and optional secrets on the cold path
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

	authToken, err := decryptOptionalSecret(appConfig, tr.AuthToken)
	if err != nil {
		log.Critical("Failed to decrypt Tele-Remote auth_token: %v", err)
		os.Exit(1)
	}

	var decryptedAllowedUsers []string
	if allowedUsers != "" {
		tokens := strings.FieldsFunc(allowedUsers, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, tok := range tokens {
			t := strings.TrimSpace(tok)
			if t == "" {
				continue
			}
			dec, decErr := appConfig.DecryptSecret(t)
			if decErr != nil {
				log.Warning("Failed to decrypt allowed user token '%s': %v (skipping)", t, decErr)
				continue
			}
			decryptedAllowedUsers = append(decryptedAllowedUsers, dec)
		}
	}
	resolvedAllowedUsers := strings.Join(decryptedAllowedUsers, ",")

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
	bot, err := core.NewBot(token, url, chatID, log, resolvedAllowedUsers)
	if err != nil {
		log.Critical("Failed to initialize Bot: %v", err)
		os.Exit(1)
	}
	if tr.BurstLimit > 0 {
		bot.SetBurstLimit(tr.BurstLimit)
	}

	// 5. Setup Routes
	routes.SetupRoutes(bot)

	// 6. Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 7. Wrap Bot methods into Subscriber Callbacks
	var initialMenuPush sync.Once
	botCallbacks := tele_interfaces.SubscriberCallbacks{
		OnTelemetry: bot.OnTelemetry,
		OnRegistration: func(clientID, componentName, menuJSON string, pub tele_interfaces.Publisher) {
			ui.OnComponentConnected(bot, clientID, componentName, menuJSON, pub)
			initialMenuPush.Do(func() {
				time.AfterFunc(2500*time.Millisecond, func() {
					if ctx.Err() == nil {
						ui.PushMainMenuToAdmins(bot)
					}
				})
			})
		},
		OnDisconnect: bot.OnDisconnect,
	}

	// 8. Initialize & Synchronously Bind gRPC Subscriber on the Cold Path
	grpcSub := subscribers.NewGrpcSubscriber(log, bindIP, bindPort)
	initialPolicy := buildTLSPolicy(appConfig, tr)
	if secErr := grpcSub.ConfigureSecurity(initialPolicy, authToken); secErr != nil {
		log.Critical("Failed to initialize Tele-Remote gRPC TLS security: %v", secErr)
		os.Exit(1)
	}
	if bindErr := grpcSub.Bind(); bindErr != nil {
		log.Critical("Failed to bind Tele-Remote gRPC listener on %s: %v", grpcAddr, bindErr)
		os.Exit(1)
	}

	appConfig.OnLiveConfUpdate(func(_ map[string]interface{}) {
		var updatedTr TeleRemoteCap
		if capErr := appConfig.Config.GetCapability("tele_remote", &updatedTr); capErr != nil {
			log.Warning("Live config update: failed to reload tele_remote capability: %v", capErr)
			return
		}

		updatedAuthToken, decErr := decryptOptionalSecret(appConfig, updatedTr.AuthToken)
		if decErr != nil {
			log.Warning("Live config update: failed to decrypt tele_remote auth_token: %v", decErr)
			return
		}

		updatedPolicy := buildTLSPolicy(appConfig, updatedTr)
		if updErr := grpcSub.UpdateRuntimeSecurity(updatedPolicy, updatedAuthToken); updErr != nil {
			log.Error("Live TLS security reload rejected (retaining previous policy): %v", updErr)
			return
		}

		if updatedTr.BurstLimit > 0 {
			bot.SetBurstLimit(updatedTr.BurstLimit)
		}

		log.Info("Live config update applied to gRPC subscriber (tls_enabled=%v, tls_mode=%s, verify_client=%v)",
			updatedPolicy.Enabled, updatedPolicy.Mode, updatedPolicy.LocalTLS.ShouldVerifyClient())
	})

	go func() {
		if err := grpcSub.StartListen(ctx, botCallbacks); err != nil && ctx.Err() == nil {
			log.Error("gRPC Subscriber serve loop terminated with error: %v", err)
			cancel()
		}
	}()
	lm.Register("gRPC_Subscriber", grpcSub.Close)
	lm.Register("Telegram_Bot", bot.Stop)

	// 9. Register tele-remote specification with web-interface dynamically (linked to lifecycle)
	_ = toolbox_spec.PublishWithLifecycle(appConfig, log, lm, "tele_remote", toolbox_spec.ServiceSpec{
		Name:        "Tele Remote",
		Tag:         "tele-remote",
		URL:         toolbox_spec.ResolveMFEURL(appConfig, fmt.Sprintf("http://%s", grpcAddr)),
		NavTitle:    "Tele Remote",
		Layer:       "L1",
		Category:    "Base Fleet",
		Priority:    40,
		Icon:        "fa-paper-plane",
		Description: "Bidirectional Telegram bridge connecting distributed microservices and worker daemons to interactive bot menus.",
		Protocol:    "gRPC Stream",
		Framing:     fmt.Sprintf("Port %d gRPC Stream", bindPort),
		DocLink:     "obsidian-brain/06-Microservices/Tele-Remote-Hub.md",
		Contracts:   buildTeleRemoteContracts(grpcAddr, url, bindPort),
	})

	// 10. Start the Bot
	go bot.Start(ctx)

	// 11. Wait for Shutdown Signals
	log.Info("Service is ready and listening for commands on %s", grpcAddr)
	lm.Wait(ctx)

	log.Info("Tele-Remote shutdown complete")
}
