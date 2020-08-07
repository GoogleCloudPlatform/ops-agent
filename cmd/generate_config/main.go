package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/Stackdriver/unified_agents/confgenerator"
)

var (
	service = flag.String("service", "", "service to generate config for")
	outDir  = flag.String("out", os.Getenv("RUNTIME_DIRECTORY"), "directory to write configuration files to")
	input   = flag.String("in", "/etc/google-ops-agent/config.yml", "path to unified agents config")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	data, err := ioutil.ReadFile(*input)
	if err != nil {
		return err
	}
	switch *service {
	case "fluentbit":
		mainConfig, parserConfig, err := confgenerator.GenerateFluentBitConfigs(data)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		path := filepath.Join(*outDir, "fluent_bit_main.conf")
		if err := ioutil.WriteFile(path, []byte(mainConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
		path = filepath.Join(*outDir, "fluent_bit_parser.conf")
		if err := ioutil.WriteFile(path, []byte(parserConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
	default:
		return fmt.Errorf("unknown service %q", *service)
	}
	return nil
}
