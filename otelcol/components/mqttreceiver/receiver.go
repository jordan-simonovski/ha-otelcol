package mqttreceiver

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

const scopeName = "github.com/local/mqttreceiver"

// mqttReceiver serves exactly one signal: either logs or metrics, depending on
// which consumer is set. Only one pipeline references the receiver at a time, so
// only one connection/subscription is created.
type mqttReceiver struct {
	cfg     *Config
	logger  *zap.Logger
	logs    consumer.Logs
	metrics consumer.Metrics
	client  mqtt.Client
}

func newLogsReceiver(cfg *Config, settings receiver.Settings, next consumer.Logs) *mqttReceiver {
	return &mqttReceiver{cfg: cfg, logger: settings.Logger, logs: next}
}

func newMetricsReceiver(cfg *Config, settings receiver.Settings, next consumer.Metrics) *mqttReceiver {
	return &mqttReceiver{cfg: cfg, logger: settings.Logger, metrics: next}
}

func (r *mqttReceiver) Start(_ context.Context, _ component.Host) error {
	scheme := "tcp"
	if r.cfg.TLS {
		scheme = "ssl"
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", scheme, r.cfg.Broker, r.cfg.Port))
	opts.SetClientID(r.effectiveClientID())
	if r.cfg.Username != "" {
		opts.SetUsername(r.cfg.Username)
	}
	if r.cfg.Password != "" {
		opts.SetPassword(r.cfg.Password)
	}
	if r.cfg.TLS {
		opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	// Reconnect and re-subscribe automatically; do not block the collector if the
	// broker is temporarily unavailable at start.
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetCleanSession(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		r.logger.Info("mqtt: connected",
			zap.String("broker", r.cfg.Broker), zap.Int("port", r.cfg.Port),
			zap.String("client_id", r.effectiveClientID()))
		r.subscribe(c)
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		r.logger.Warn("mqtt connection lost", zap.Error(err))
	})

	r.client = mqtt.NewClient(opts)

	token := r.client.Connect()
	timeout := r.cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if !token.WaitTimeout(timeout) {
		// Connection still pending; ConnectRetry will keep trying in the background.
		r.logger.Warn("mqtt initial connection pending; retrying in background",
			zap.String("broker", r.cfg.Broker), zap.Int("port", r.cfg.Port))
		return nil
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: failed to connect to %s:%d: %w", r.cfg.Broker, r.cfg.Port, err)
	}
	return nil
}

// effectiveClientID disambiguates the MQTT client ID per signal so that a logs
// instance and a metrics instance never collide on the broker (brokers evict an
// existing session when a new connection reuses its client ID).
func (r *mqttReceiver) effectiveClientID() string {
	if r.metrics != nil {
		return r.cfg.ClientID + "-metrics"
	}
	return r.cfg.ClientID + "-logs"
}

func (r *mqttReceiver) subscribe(c mqtt.Client) {
	filters := make(map[string]byte, len(r.cfg.Topics))
	for _, t := range r.cfg.Topics {
		filters[t] = byte(r.cfg.QoS)
	}
	token := c.SubscribeMultiple(filters, r.handleMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		r.logger.Error("mqtt: failed to subscribe", zap.Strings("topics", r.cfg.Topics), zap.Error(err))
		return
	}
	r.logger.Info("mqtt: subscribed", zap.Strings("topics", r.cfg.Topics))
}

func (r *mqttReceiver) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	if r.metrics != nil {
		r.consumeMetrics(msg)
		return
	}
	r.consumeLogs(msg)
}

func (r *mqttReceiver) consumeLogs(msg mqtt.Message) {
	logs := plog.NewLogs()
	sl := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
	sl.Scope().SetName(scopeName)

	lr := sl.LogRecords().AppendEmpty()
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	lr.Body().SetStr(string(msg.Payload()))

	attrs := lr.Attributes()
	attrs.PutStr("mqtt.topic", msg.Topic())
	attrs.PutInt("mqtt.qos", int64(msg.Qos()))
	attrs.PutBool("mqtt.retained", msg.Retained())
	attrs.PutInt("mqtt.message_id", int64(msg.MessageID()))

	if err := r.logs.ConsumeLogs(context.Background(), logs); err != nil {
		r.logger.Error("mqtt: failed to forward message to pipeline", zap.Error(err))
	}
}

func (r *mqttReceiver) consumeMetrics(msg mqtt.Message) {
	md, n := decodeMetrics(msg.Payload(), msg.Topic(), pcommon.NewTimestampFromTime(time.Now()))
	if n == 0 {
		r.logger.Debug("mqtt: no numeric values found in message; skipped",
			zap.String("topic", msg.Topic()))
		return
	}
	if err := r.metrics.ConsumeMetrics(context.Background(), md); err != nil {
		r.logger.Error("mqtt: failed to forward metrics to pipeline", zap.Error(err))
	}
}

// decodeMetrics turns an MQTT payload into gauge data points. It accepts:
//   - a JSON object: every numeric (or bool) leaf becomes a gauge named by its
//     (dotted) key path; nested objects are flattened.
//   - a bare JSON number or bool: one gauge named after the topic.
//   - a plain numeric string (e.g. "21.5"): one gauge named after the topic.
//
// Non-numeric values are skipped. The returned int is the number of data points
// produced; 0 means nothing usable was found.
func decodeMetrics(payload []byte, topic string, ts pcommon.Timestamp) (pmetric.Metrics, int) {
	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(scopeName)

	count := 0
	add := func(name string, value float64) {
		m := sm.Metrics().AppendEmpty()
		m.SetName(name)
		dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(ts)
		dp.SetDoubleValue(value)
		dp.Attributes().PutStr("mqtt.topic", topic)
		count++
	}

	trimmed := bytes.TrimSpace(payload)
	var parsed any
	if len(trimmed) > 0 && json.Unmarshal(trimmed, &parsed) == nil {
		switch v := parsed.(type) {
		case float64:
			add(metricNameFromTopic(topic), v)
		case bool:
			add(metricNameFromTopic(topic), boolToFloat(v))
		case map[string]any:
			flattenJSON("", v, add)
		}
	} else if f, err := strconv.ParseFloat(string(trimmed), 64); err == nil {
		add(metricNameFromTopic(topic), f)
	}

	return md, count
}

func flattenJSON(prefix string, obj map[string]any, add func(string, float64)) {
	for k, val := range obj {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		switch v := val.(type) {
		case float64:
			add(name, v)
		case bool:
			add(name, boolToFloat(v))
		case map[string]any:
			flattenJSON(name, v, add)
		}
	}
}

func metricNameFromTopic(topic string) string {
	name := strings.ReplaceAll(topic, "/", ".")
	name = strings.Trim(name, ".")
	if name == "" {
		return "mqtt"
	}
	return name
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (r *mqttReceiver) Shutdown(_ context.Context) error {
	if r.client != nil && r.client.IsConnectionOpen() {
		r.client.Disconnect(250)
	}
	return nil
}
