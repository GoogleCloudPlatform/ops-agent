package ast

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnquoteString(t *testing.T) {
	for in, out := range map[string]string{
		`\150\145\154\154\157`:     "\150\145\154\154\157",
		`\x48\x65\x6C\x6C\x6F`:     "Hello",
		`\150\145\u013E\u013E\157`: "\150\145\u013E\u013E\157",
		`sl\\as\\\\h`:              `sl\as\\h`,
		`\777`:                     `?7`,
		`\377`:                     "\u00FF",
		`\`:                        `\`,
		`☃`:                        `☃`,
	} {
		in, out := in, out
		t.Run(in, func(t *testing.T) {
			got, err := UnquoteString(in)
			if got != out {
				t.Errorf("got %q, want %q", got, out)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func TestValidPath(t *testing.T) {
	for _, test := range []struct {
		in            Target
		want          string
		fluentBitPath []string
	}{
		{
			Target{"jsonPayload", "hello"},
			"jsonPayload.hello",
			[]string{"hello"},
		},
		{
			Target{`"json\u0050ayload"`, "hello"},
			"jsonPayload.hello",
			[]string{"hello"},
		},
		{
			Target{"severity"},
			"severity",
			[]string{"logging.googleapis.com/severity"},
		},
		{
			Target{"httpRequest", "status"},
			"httpRequest.status",
			[]string{"logging.googleapis.com/httpRequest", "status"},
		},
		{
			Target{"sourceLocation", "line"},
			"sourceLocation.line",
			[]string{"logging.googleapis.com/sourceLocation", "line"},
		},
		{
			Target{"labels", "custom"},
			"labels.custom",
			[]string{"logging.googleapis.com/labels", "custom"},
		},
		{
			Target{`jsonPayload`, `"escaped fields \a\b\f\n\r\t\v"`},
			`jsonPayload."escaped\u0020fields\u0020\a\b\f\n\r\t\v"`,
			[]string{"escaped fields \a\b\f\n\r\t\v"},
		},
	} {
		test := test
		t.Run(test.in.String(), func(t *testing.T) {
			got := test.in.String()
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("unexpected target string (got -/want +):\n%s", diff)
			}
			gotPath, err := test.in.fluentBitPath()
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
			}
			if diff := cmp.Diff(gotPath, test.fluentBitPath); diff != "" {
				t.Errorf("unexpected fluent-bit path (got -/want +):\n%s", diff)
			}
		})
	}
}

func TestInvalidFluentBitPath(t *testing.T) {
	for _, test := range []Target{
		{"notJsonPayload"},
		{"jsonPayload"},
		{"sourceLocation"},
		{"jsonPayload", `"broken\descape"`},
	} {
		test := test
		t.Run(test.String(), func(t *testing.T) {
			got, err := test.fluentBitPath()
			if err == nil {
				t.Errorf("got unexpected success for %v: %+v", test, got)
			}
		})
	}
}
