package grpc_control

import _ "embed"

// TeleRemoteProtoSchema embeds the verbatim gRPC bidirectional streaming contract.
//
//go:embed teleremote.proto
var TeleRemoteProtoSchema string

