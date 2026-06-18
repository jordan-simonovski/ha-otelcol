package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Options mirrors the add-on schema in otelcol/config.yaml. Pointers are used
// where "absent" must be distinguished from a zero value so defaults match the
// previous bash behavior.
type Options struct {
	LogLevel    string        `json:"log_level"`
	Processors  ProcessorsOpt `json:"processors"`
	MQTT        *MQTTOpt      `json:"mqtt"`
	Exporters   []ExporterOpt `json:"exporters"`
	ExtraConfig string        `json:"extra_config"`
}

type ProcessorsOpt struct {
	Batch *BatchOpt `json:"batch"`
}

type BatchOpt struct {
	Timeout       string `json:"timeout"`
	SendBatchSize *int   `json:"send_batch_size"`
	SendBatchMax  *int   `json:"send_batch_max_size"`
}

type MQTTOpt struct {
	Broker   string   `json:"broker"`
	Port     *int     `json:"port"`
	QoS      *int     `json:"qos"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	ClientID string   `json:"client_id"`
	TLS      bool     `json:"tls"`
	Signal   string   `json:"signal"`
	Topics   []string `json:"topics"`
}

type ExporterOpt struct {
	Type     string   `json:"type"`
	Endpoint string   `json:"endpoint"`
	Headers  []string `json:"headers"`
	Database string   `json:"database"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      string   `json:"ttl"`
	Timeout  string   `json:"timeout"`
}

// decodeOptions reads one JSON object from r. A malformed or empty stream is an
// error; the collector cannot start without a config.
func decodeOptions(r io.Reader) (*Options, error) {
	var o Options
	if err := json.NewDecoder(r).Decode(&o); err != nil {
		return nil, fmt.Errorf("parsing options: %w", err)
	}
	return &o, nil
}
