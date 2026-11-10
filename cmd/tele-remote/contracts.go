package main

import (
	"strings"

	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
)

func buildTeleRemoteContracts(grpcAddr, botURL string, bindPort int) []toolbox_spec.ExchangeContract {
	grpcOps := toolbox_spec.ExtractProtoTextOperations(grpc_control.TeleRemoteProtoSchema)
	if len(grpcOps) == 0 {
		grpcOps = []toolbox_spec.ContractOperation{
			{
				Name:        "Connect",
				Kind:        "BIDI_STREAM",
				Target:      "teleremote.TeleRemoteService/Connect",
				Input:       "stream ComponentMessage",
				Output:      "stream BotCommand",
				Description: "Bidirectional gRPC stream carrying menus/telemetry upstream and BotCommands downstream",
			},
		}
	}

	return []toolbox_spec.ExchangeContract{
		{
			ID:        "grpc-bidi-stream",
			Type:      "gRPC Bidirectional Stream",
			TabLabel:  "gRPC Stream (teleremote.proto)",
			Icon:      "fa-exchange",
			Direction: "bidirectional",
			Transport: "HTTP/2 + Protobuf Stream",
			Address:   toolbox_spec.DisplayAddress(grpcAddr),
			Port:      bindPort,
			Framing:   "teleremote.TeleRemoteService/Connect (ComponentMessage <-> BotCommand)",
			Auth:      "Component identity registered on initial stream frame",
			Summary:   "Full-duplex gRPC stream allowing ecosystem microservices to push dynamic menu trees & telemetry and receive Telegram interactive BotCommand invocations.",
			Source:    "tele-remote/src/grpc_control/teleremote.proto (go:embed)",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "watchdog_agent", Role: "client", Description: "Publishes supervised service controls & restart triggers"},
				{Service: "config_server", Role: "client", Description: "Publishes hierarchical config browser, edit, reload, and persist menus"},
				{Service: "notif_server", Role: "client", Description: "Publishes alerting test dispatch & provider configuration menus"},
				{Service: "rag_engine", Role: "client", Description: "Publishes index/sync commands & brain telemetry"},
				{Service: "web_interface", Role: "observer", Description: "Surfaces tele-remote status and exchange contract metadata"},
			},
			Operations:   grpcOps,
			SchemaFormat: "proto3",
			Schema:       grpc_control.TeleRemoteProtoSchema,
		},
		{
			ID:        "telegram-bot-api",
			Type:      "Telegram Bot API",
			TabLabel:  "Telegram Bot Gateway",
			Icon:      "fa-paper-plane",
			Direction: "outbound",
			Transport: "HTTPS JSON Long-Poll / Webhook Client (telebot.v3)",
			Address:   botURL,
			Framing:   "Telegram Bot API updates (/start, inline Reply & Callback menus)",
			Auth:      "Decrypted bot token + chat_id via AppConfig.DecryptSecret",
			Summary:   "Polls Telegram Bot API updates and dispatches user button callbacks and input prompts back into connected gRPC subscriber streams.",
			Source:    "tele-remote/src/telegram/routes/routes.go + ui/menus.go",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "Telegram Bot API", Role: "external_gateway", Description: "Operator chat interface executing microservice actions"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "CmdStart", Kind: "COMMAND", Target: "/start", Output: "Reply keyboard + connected component buttons", Description: "Renders interactive control menu root"},
				{Name: "PowerOff", Kind: "BUTTON", Target: "POWER_OFF (1)", Output: "BotCommand { CommandType: POWER_OFF }", Description: "Broadcasts emergency power-off command across connected clients"},
				{Name: "CloseAll", Kind: "BUTTON", Target: "CLOSE_ALL_POSITIONS (2)", Output: "BotCommand { CommandType: CLOSE_ALL_POSITIONS }", Description: "Broadcasts position flatten command"},
				{Name: "DynamicAction", Kind: "CALLBACK", Target: "tb.OnText / tb.OnCallback", Input: "Operator text or inline callback", Output: "BotCommand { CUSTOM_COMMAND (99), custom_payload, input }", Description: "Routes dynamic microservice submenu action or input prompt"},
			},
			SchemaFormat: "wire",
			Schema: strings.Join([]string{
				"Telegram Command Handlers (tele-remote/src/telegram/routes/routes.go):",
				"  /start       -> Render connected component root buttons",
				"  Power Off    -> Dispatches BotCommand.POWER_OFF (1)",
				"  Close All    -> Dispatches BotCommand.CLOSE_ALL_POSITIONS (2)",
				"  tb.OnText    -> Routes reply-keyboard & pending input prompts",
				"  tb.OnCallback-> Executes ComponentMenu dynamic callback IDs via gRPC publisher",
			}, "\n"),
		},
	}
}
