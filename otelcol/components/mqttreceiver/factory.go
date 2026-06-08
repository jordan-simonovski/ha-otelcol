package mqttreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

var componentType = component.MustNewType("mqtt")

const stability = component.StabilityLevelAlpha

// NewFactory returns a factory for the MQTT receiver. The receiver can feed
// either a metrics or a logs pipeline; the add-on wires it into exactly one.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		componentType,
		createDefaultConfig,
		receiver.WithMetrics(createMetrics, stability),
		receiver.WithLogs(createLogs, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Port:           1883,
		QoS:            0,
		ClientID:       "otelcol-ha",
		ConnectTimeout: 10 * time.Second,
	}
}

func createMetrics(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	next consumer.Metrics,
) (receiver.Metrics, error) {
	return newMetricsReceiver(cfg.(*Config), settings, next), nil
}

func createLogs(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	next consumer.Logs,
) (receiver.Logs, error) {
	return newLogsReceiver(cfg.(*Config), settings, next), nil
}
