package ast

import (
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
)

func TestParseTextString(t *testing.T) {
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
			got, err := ParseText(&token.Token{Lit: []byte(in)})
			if got != out {
				t.Errorf("got %q, want %q", got, out)
			}
			if err != nil {
				t.Error(err)
			}

			in = `"` + in + `"`
			got, err = ParseString(&token.Token{Lit: []byte(in)})
			if got != out {
				t.Errorf("got %q, want %q", got, out)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}
