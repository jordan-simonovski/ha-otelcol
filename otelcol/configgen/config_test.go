package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "regenerate golden files")

// TestGenerateGolden runs every valid fixture through the generator and pins
// the output to a checked-in golden. Run with -update to regenerate goldens.
func TestGenerateGolden(t *testing.T) {
	fixtures, err := filepath.Glob("../tests/config/fixtures/valid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no valid fixtures found")
	}

	for _, fx := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fx), ".json")
		t.Run(name, func(t *testing.T) {
			in, err := os.Open(fx)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()

			opts, err := decodeOptions(in)
			if err != nil {
				t.Fatalf("decode %s: %v", fx, err)
			}
			cfg, _ := buildConfig(opts)
			got, err := yaml.Marshal(cfg)
			if err != nil {
				t.Fatalf("marshal %s: %v", fx, err)
			}

			golden := filepath.Join("testdata", "golden", name+".yaml")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run `go test -update`?): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("config mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}

// TestDecodeMalformedJSON proves the generator's own error path: bad JSON is an
// error, not a silently-empty config.
func TestDecodeMalformedJSON(t *testing.T) {
	if _, err := decodeOptions(strings.NewReader("{")); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
