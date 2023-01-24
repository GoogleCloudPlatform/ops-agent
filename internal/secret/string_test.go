package secret_test

import (
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
	"github.com/goccy/go-yaml"
)

func TestSecretStringStringer(t *testing.T) {
	var s secret.String = "My credit card number!"
	result := s.String()
	if !strings.Contains(result, "x") {
		t.Fatalf("expected result to be redacted, instead was \"%s\"", result)
	}
}

func TestSecretStringMarshalYAML(t *testing.T) {
	type x struct {
		S secret.String `yaml:"s"`
	}

	testX := x{S: "My credit card number!"}
	result, err := yaml.Marshal(testX)
	if err != nil {
		t.Fatalf("expected marshal not to error, got: %s", result)
	}
	if string(result) != "s: xxxxx\n" {
		t.Fatalf("expected secret field to be redacted, got: %s", string(result))
	}
}
