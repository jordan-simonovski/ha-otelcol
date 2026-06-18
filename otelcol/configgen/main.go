package main

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultOptionsPath = "/data/options.json"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "otelcol-config:", err)
		os.Exit(1)
	}
}

// run reads options from args[0] (default /data/options.json), writes the
// generated YAML to stdout and warnings to stderr. Any error is fatal: the
// collector must not start without a config.
func run(args []string, stdout, stderr io.Writer) error {
	path := defaultOptionsPath
	if len(args) > 0 && args[0] != "" {
		path = args[0]
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	opts, err := decodeOptions(f)
	if err != nil {
		return err
	}

	cfg, warnings := buildConfig(opts)
	for _, w := range warnings {
		fmt.Fprintln(stderr, w)
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	_, err = stdout.Write(out)
	return err
}
