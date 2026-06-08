package mqttreceiver

import (
	"errors"
	"time"
)

// Config defines the configuration for the MQTT receiver.
type Config struct {
	// Broker is the hostname or IP of the MQTT broker (without scheme or port).
	Broker string `mapstructure:"broker"`
	// Port is the broker TCP port. Defaults to 1883 (8883 is typical for TLS).
	Port int `mapstructure:"port"`
	// Topics is the list of topic filters to subscribe to.
	Topics []string `mapstructure:"topics"`
	// QoS is the MQTT quality-of-service level (0, 1, or 2).
	QoS int `mapstructure:"qos"`
	// Username for broker authentication. Optional.
	Username string `mapstructure:"username"`
	// Password for broker authentication. Optional.
	Password string `mapstructure:"password"`
	// ClientID is the MQTT client identifier.
	ClientID string `mapstructure:"client_id"`
	// TLS enables a TLS connection to the broker.
	TLS bool `mapstructure:"tls"`
	// ConnectTimeout bounds the initial connection attempt.
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
}

func (c *Config) Validate() error {
	if c.Broker == "" {
		return errors.New("mqtt: 'broker' must be specified")
	}
	if len(c.Topics) == 0 {
		return errors.New("mqtt: at least one entry in 'topics' is required")
	}
	if c.QoS < 0 || c.QoS > 2 {
		return errors.New("mqtt: 'qos' must be 0, 1, or 2")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("mqtt: 'port' must be in range 1-65535")
	}
	return nil
}
