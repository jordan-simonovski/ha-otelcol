package mqttreceiver

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func collectGauges(t *testing.T, payload, topic string) map[string]float64 {
	t.Helper()
	md, n := decodeMetrics([]byte(payload), topic, pcommon.NewTimestampFromTime(time.Now()))
	out := make(map[string]float64)
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		sms := rms.At(i).ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			ms := sms.At(j).Metrics()
			for k := 0; k < ms.Len(); k++ {
				m := ms.At(k)
				dps := m.Gauge().DataPoints()
				for d := 0; d < dps.Len(); d++ {
					out[m.Name()] = dps.At(d).DoubleValue()
					topicAttr, ok := dps.At(d).Attributes().Get("mqtt.topic")
					if !ok || topicAttr.Str() != topic {
						t.Fatalf("metric %q missing mqtt.topic=%q attribute", m.Name(), topic)
					}
				}
			}
		}
	}
	if len(out) != n {
		t.Fatalf("count mismatch: returned n=%d but found %d metrics", n, len(out))
	}
	return out
}

func TestDecodeJSONObject(t *testing.T) {
	got := collectGauges(t,
		`{"temperature":21.5,"humidity":60,"on":true,"state":"ON","nested":{"battery":90}}`,
		"zigbee2mqtt/sensor")

	want := map[string]float64{
		"temperature":    21.5,
		"humidity":       60,
		"on":             1,
		"nested.battery": 90,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d metrics, got %d: %v", len(want), len(got), got)
	}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("metric %q = %v, want %v", name, got[name], v)
		}
	}
	if _, ok := got["state"]; ok {
		t.Errorf("non-numeric field 'state' should have been skipped")
	}
}

func TestDecodeBareNumber(t *testing.T) {
	got := collectGauges(t, "21.5", "home/livingroom/temp")
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d: %v", len(got), got)
	}
	if v := got["home.livingroom.temp"]; v != 21.5 {
		t.Errorf("expected gauge home.livingroom.temp=21.5, got %v", v)
	}
}

func TestDecodePlainNumericString(t *testing.T) {
	// Not valid JSON but a parseable float (e.g. with surrounding whitespace).
	got := collectGauges(t, "  42 ", "a/b")
	if v, ok := got["a.b"]; !ok || v != 42 {
		t.Errorf("expected a.b=42, got %v (present=%v)", v, ok)
	}
}

func TestDecodeNonNumericSkipped(t *testing.T) {
	_, n := decodeMetrics([]byte("ON"), "x/y", pcommon.NewTimestampFromTime(time.Now()))
	if n != 0 {
		t.Errorf("expected 0 metrics for non-numeric payload, got %d", n)
	}
}

func TestDecodeEmptySkipped(t *testing.T) {
	_, n := decodeMetrics([]byte("   "), "x/y", pcommon.NewTimestampFromTime(time.Now()))
	if n != 0 {
		t.Errorf("expected 0 metrics for empty payload, got %d", n)
	}
}
