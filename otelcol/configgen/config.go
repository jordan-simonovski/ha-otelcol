package main

import (
	"fmt"
	"strings"
)

const (
	defaultMQTTPort       = 1883
	defaultMQTTClientID   = "otelcol-ha"
	defaultFileExportPath = "/share/otelcol/telemetry.json"
	defaultPromEndpoint   = "0.0.0.0:8889"
)

// Config is the generated middle layer. yaml.v3 marshals struct fields in
// declaration order; map keys are emitted sorted. Order is cosmetic -- the
// collector does not care -- and golden tests pin whatever the marshaler emits.
type Config struct {
	Receivers  map[string]any `yaml:"receivers"`
	Processors map[string]any `yaml:"processors"`
	Exporters  map[string]any `yaml:"exporters"`
	Extensions map[string]any `yaml:"extensions"`
	Service    Service        `yaml:"service"`
}

type Service struct {
	Extensions []string            `yaml:"extensions,flow"`
	Pipelines  map[string]Pipeline `yaml:"pipelines"`
}

type Pipeline struct {
	Receivers  []string `yaml:"receivers,flow"`
	Processors []string `yaml:"processors,flow"`
	Exporters  []string `yaml:"exporters,flow"`
}

type endpointHolder struct {
	Endpoint string `yaml:"endpoint"`
}

type otlpReceiver struct {
	Protocols otlpProtocols `yaml:"protocols"`
}

type otlpProtocols struct {
	GRPC endpointHolder `yaml:"grpc"`
	HTTP endpointHolder `yaml:"http"`
}

type mqttReceiver struct {
	Broker   string   `yaml:"broker"`
	Port     int      `yaml:"port"`
	QoS      int      `yaml:"qos"`
	ClientID string   `yaml:"client_id"`
	TLS      bool     `yaml:"tls"`
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
	Topics   []string `yaml:"topics"`
}

type memoryLimiter struct {
	CheckInterval        string `yaml:"check_interval"`
	LimitPercentage      int    `yaml:"limit_percentage"`
	SpikeLimitPercentage int    `yaml:"spike_limit_percentage"`
}

// batchProcessor with all fields unset marshals to "{}".
type batchProcessor struct {
	Timeout       string `yaml:"timeout,omitempty"`
	SendBatchSize *int   `yaml:"send_batch_size,omitempty"`
	SendBatchMax  *int   `yaml:"send_batch_max_size,omitempty"`
}

type otlpExporter struct {
	Endpoint string            `yaml:"endpoint"`
	Headers  map[string]string `yaml:"headers,omitempty"`
}

type prometheusExporter struct {
	Endpoint string `yaml:"endpoint"`
}

type fileExporter struct {
	Path string `yaml:"path"`
}

type clickhouseExporter struct {
	Endpoint string `yaml:"endpoint"`
	Database string `yaml:"database,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	TTL      string `yaml:"ttl,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

type debugExporter struct {
	Verbosity string `yaml:"verbosity"`
}

// buildConfig applies the same rules the bash run script used. The []string is
// diagnostic warnings for stderr; building never fails.
func buildConfig(o *Options) (*Config, []string) {
	var warnings []string

	receivers := map[string]any{
		"otlp": otlpReceiver{Protocols: otlpProtocols{
			GRPC: endpointHolder{Endpoint: "0.0.0.0:4317"},
			HTTP: endpointHolder{Endpoint: "0.0.0.0:4318"},
		}},
	}

	mqttEnabled := false
	mqttSignal := "metrics"
	if o.MQTT != nil && o.MQTT.Broker != "" {
		mqttEnabled = true
		m := o.MQTT
		if m.Signal == "logs" {
			mqttSignal = "logs"
		}
		port := defaultMQTTPort
		if m.Port != nil && *m.Port > 0 {
			port = *m.Port
		}
		qos := 0
		if m.QoS != nil && *m.QoS >= 0 && *m.QoS <= 2 {
			qos = *m.QoS
		}
		clientID := m.ClientID
		if clientID == "" {
			clientID = defaultMQTTClientID
		}
		topics := make([]string, 0, len(m.Topics))
		for _, t := range m.Topics {
			if t != "" {
				topics = append(topics, t)
			}
		}
		receivers["mqtt"] = mqttReceiver{
			Broker:   m.Broker,
			Port:     port,
			QoS:      qos,
			ClientID: clientID,
			TLS:      m.TLS,
			Username: m.Username,
			Password: m.Password,
			Topics:   topics,
		}
		warnings = append(warnings, fmt.Sprintf("MQTT receiver enabled for broker %s", m.Broker))
	} else {
		warnings = append(warnings, "MQTT receiver disabled (no broker configured)")
	}

	processors := map[string]any{
		"memory_limiter": memoryLimiter{
			CheckInterval:        "5s",
			LimitPercentage:      80,
			SpikeLimitPercentage: 25,
		},
		"batch": buildBatch(o.Processors.Batch),
	}

	exporters := map[string]any{}
	var signalExporters, metricExporters []string

	addDebug := func(name string) {
		exporters[name] = debugExporter{Verbosity: "basic"}
		signalExporters = appendUnique(signalExporters, name)
		metricExporters = appendUnique(metricExporters, name)
	}

	if len(o.Exporters) == 0 {
		addDebug("debug")
	} else {
		for i, e := range o.Exporters {
			name := fmt.Sprintf("%s/%d", e.Type, i)
			switch e.Type {
			case "otlp", "otlphttp":
				if e.Endpoint == "" {
					warnings = append(warnings, fmt.Sprintf("Exporter %s has no endpoint; skipping.", name))
					continue
				}
				exporters[name] = otlpExporter{Endpoint: e.Endpoint, Headers: parseHeaders(e.Headers)}
				signalExporters = append(signalExporters, name)
				metricExporters = append(metricExporters, name)
			case "prometheus":
				ep := e.Endpoint
				if ep == "" {
					ep = defaultPromEndpoint
				}
				exporters[name] = prometheusExporter{Endpoint: ep}
				metricExporters = append(metricExporters, name)
			case "file":
				ep := e.Endpoint
				if ep == "" {
					ep = defaultFileExportPath
				}
				exporters[name] = fileExporter{Path: ep}
				signalExporters = append(signalExporters, name)
				metricExporters = append(metricExporters, name)
			case "clickhouse":
				if e.Endpoint == "" {
					warnings = append(warnings, fmt.Sprintf("Exporter %s has no endpoint; skipping.", name))
					continue
				}
				exporters[name] = clickhouseExporter{
					Endpoint: e.Endpoint,
					Database: e.Database,
					Username: e.Username,
					Password: e.Password,
					TTL:      e.TTL,
					Timeout:  e.Timeout,
				}
				signalExporters = append(signalExporters, name)
				metricExporters = append(metricExporters, name)
			case "debug":
				exporters[name] = debugExporter{Verbosity: "basic"}
				signalExporters = append(signalExporters, name)
				metricExporters = append(metricExporters, name)
			default:
				warnings = append(warnings, fmt.Sprintf("Unknown exporter type '%s'; skipping.", e.Type))
			}
		}
	}

	if len(signalExporters) == 0 && len(metricExporters) == 0 {
		warnings = append(warnings, "No valid exporters configured; falling back to debug.")
		addDebug("debug")
	}

	if mqttEnabled && mqttSignal == "logs" && len(signalExporters) == 0 {
		warnings = append(warnings, "MQTT signal is 'logs' but no logs-capable exporter is configured; adding debug.")
		addDebug("debug")
	}

	extensions := map[string]any{
		"pprof":  endpointHolder{Endpoint: "127.0.0.1:1777"},
		"zpages": endpointHolder{Endpoint: "127.0.0.1:55679"},
	}

	pipelines := map[string]Pipeline{}
	if len(signalExporters) > 0 {
		pipelines["traces"] = Pipeline{
			Receivers:  []string{"otlp"},
			Processors: []string{"memory_limiter", "batch"},
			Exporters:  signalExporters,
		}
		logsReceivers := []string{"otlp"}
		if mqttEnabled && mqttSignal == "logs" {
			logsReceivers = []string{"otlp", "mqtt"}
		}
		pipelines["logs"] = Pipeline{
			Receivers:  logsReceivers,
			Processors: []string{"memory_limiter", "batch"},
			Exporters:  signalExporters,
		}
	}
	if len(metricExporters) > 0 {
		metricsReceivers := []string{"otlp"}
		if mqttEnabled && mqttSignal == "metrics" {
			metricsReceivers = []string{"otlp", "mqtt"}
		}
		pipelines["metrics"] = Pipeline{
			Receivers:  metricsReceivers,
			Processors: []string{"memory_limiter", "batch"},
			Exporters:  metricExporters,
		}
	}

	return &Config{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: extensions,
		Service: Service{
			Extensions: []string{"health_check", "pprof", "zpages"},
			Pipelines:  pipelines,
		},
	}, warnings
}

func buildBatch(b *BatchOpt) batchProcessor {
	if b == nil {
		return batchProcessor{}
	}
	return batchProcessor{
		Timeout:       b.Timeout,
		SendBatchSize: b.SendBatchSize,
		SendBatchMax:  b.SendBatchMax,
	}
}

// parseHeaders turns ["k=v", ...] into a map. Entries without "=" or with an
// empty key are dropped. Returns nil when nothing valid remains so the field is
// omitted.
func parseHeaders(hs []string) map[string]string {
	m := make(map[string]string, len(hs))
	for _, h := range hs {
		k, v, found := strings.Cut(h, "=")
		if !found || k == "" {
			continue
		}
		m[k] = v
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
