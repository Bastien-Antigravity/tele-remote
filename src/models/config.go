package models

import "time"

// -----------------------------------------------------------------------------
// Core Application and Configuration Structs
// -----------------------------------------------------------------------------

// MConfig is the root structure for the application's configuration.
type MConfig struct {
	Name      string `yaml:"name"`
	Host      string `yaml:"host"`
	Http_Port int    `yaml:"http_port"`
	LogLevel  string `yaml:"log_level"`
	GRPC_Host string `yaml:"grpc_host"`
	GRPC_Port int    `yaml:"grpc_port"`

	// Sub-configuration sections
	DataSources []*MDataSourceConfig `yaml:"data_sources"`
	NATS        MNATSConfig          `yaml:"nats"`
}

// -----------------------------------------------------------------------------
// Transport and Data Source Structs
// -----------------------------------------------------------------------------

// MConnectionConfig holds the base connection details for transport clients (e.g., WebSocketClient).
type MConnectionConfig struct {
	Endpoint          string        `yaml:"endpoint"`
	ReconnectAttempts int           `yaml:"reconnect_attempts"`
	ReconnectDelay    time.Duration `yaml:"reconnect_delay"`
}

// -----------------------------------------------------------------------------

// MDataSourceConfig holds configuration for a single external data feed (e.g., Binance).
type MDataSourceConfig struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Endpoint string   `yaml:"endpoint"`
	Symbols  []string `yaml:"symbols"`
	APIKey   string   `yaml:"api_key,omitempty"`

	// Separate struct for transport-specific settings
	ConnectionConfig *MConnectionConfig `yaml:"connection_config"`
}

// -----------------------------------------------------------------------------
// NATS and JetStream Structs
// -----------------------------------------------------------------------------

// MNATSConfig holds configuration for the NATS client connection.
type MNATSConfig struct {
	Servers   []string `yaml:"servers"`
	Subject   string   `yaml:"subject"`
	ClusterID string   `yaml:"cluster_id"`
	ClientID  string   `yaml:"client_id"`

	// Critical for connection reliability
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	ReconnectWait  time.Duration `yaml:"reconnect_wait"`
	MaxReconnects  int           `yaml:"max_reconnects"`
	FlushTimeout   time.Duration `yaml:"flush_timeout"`

	// For subject flexibility
	SubjectPrefix string `yaml:"subject_prefix"`

	// JetStream Configuration
	JetStream *MJetStreamConfig `yaml:"jetstream"`
}

// -----------------------------------------------------------------------------

// MJetStreamConfig holds specific settings for stream creation and publishing.
type MJetStreamConfig struct {
	// Core JetStream settings
	Enabled    bool   `yaml:"enabled"`
	StreamName string `yaml:"stream_name"`

	// Stream configuration
	Subjects        []string `yaml:"subjects"`
	RetentionPolicy string   `yaml:"retention"`
	Storage         string   `yaml:"storage"`
	Replicas        int      `yaml:"replicas"`

	// Stream limits
	MaxAge     time.Duration `yaml:"max_age"`
	MaxMsgs    int64         `yaml:"max_msgs"`
	MaxBytes   int64         `yaml:"max_bytes"`
	MaxMsgSize int32         `yaml:"max_msg_size"`

	// Consumer configuration (if needed by the producer for setup)
	DurableConsumer string        `yaml:"durable_consumer"`
	AckWait         time.Duration `yaml:"ack_wait"`
}
