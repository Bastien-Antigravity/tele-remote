package main

import (
	"strings"
	"testing"
)

func TestBuildTeleRemoteContracts(t *testing.T) {
	contracts := buildTeleRemoteContracts("127.0.0.1:1863", "https://api.telegram.org", 1863)
	if len(contracts) != 2 {
		t.Fatalf("expected 2 tele-remote contracts, got %d", len(contracts))
	}
	if contracts[0].ID != "grpc-bidi-stream" || !strings.Contains(contracts[0].Schema, "service TeleRemoteService") {
		t.Fatalf("expected embedded teleremote.proto schema, got: %+v", contracts[0])
	}
	if len(contracts[0].Operations) == 0 || contracts[0].Operations[0].Kind != "BIDI_STREAM" {
		t.Fatalf("expected BIDI_STREAM Connect operation, got: %+v", contracts[0].Operations)
	}
	if contracts[1].ID != "telegram-bot-api" || len(contracts[1].Operations) < 4 {
		t.Fatalf("unexpected telegram contract: %+v", contracts[1])
	}
}

