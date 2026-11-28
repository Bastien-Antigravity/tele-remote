package grpc_control

/*
ESSENTIAL PROCESS:
Embeds the canonical teleremote.proto gRPC bidirectional streaming contract
at compile time for live OpenMFE ExchangeContract introspection.

DATA FLOW:
1. Go embed directive loads teleremote.proto into TeleRemoteProtoSchema.
2. cmd/tele-remote/contracts.go parses and publishes the embedded schema to web-interface.

KEY PARAMETERS:
- TeleRemoteProtoSchema: Raw proto3 IDL string for TeleRemoteService.
*/

import _ "embed"

// -----------------------------------------------------------------------------

// TeleRemoteProtoSchema embeds the verbatim gRPC bidirectional streaming contract.
//
//go:embed teleremote.proto
var TeleRemoteProtoSchema string
