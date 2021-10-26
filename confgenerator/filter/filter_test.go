package filter

import (
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/lexer"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/parser"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
)

var validFilters = []string{
	`"this is a simple quoted string"`,
	`foo."bar"`,
	`foo = "hello"`,
	`foo."bar.baz" = "hello"`,
	`a.b.c=~"b.*c"`,
	`-a < 1`,
	`NOT a > 3`,
	`(foo.bar = "one" OR foo.bar = "two") foo.baz = "three"`,
	`foo.one = 1 foo.two = 2 AND foo.three = 3`,
	`int_field:0 OR int_field:0 AND int_field:0`,
	`compound.string_field : egg"men"`,
	`compound.string_field : wal\"rus`,
}

func TestShouldLex(t *testing.T) {
	for _, test := range validFilters {
		test := test
		t.Run(test, func(t *testing.T) {
			l := lexer.NewLexer([]byte(test))
			for tok := l.Scan(); tok.Type != token.EOF; tok = l.Scan() {
				if tok.Type == token.INVALID {
					t.Errorf("got invalid token: %v", token.TokMap.TokenString(tok))
				}
				t.Logf("tok: %v", token.TokMap.TokenString(tok))
			}
		})
	}
}

func TestShouldParse(t *testing.T) {
	for _, test := range validFilters {
		test := test
		t.Run(test, func(t *testing.T) {
			lex := lexer.NewLexer([]byte(test))
			p := parser.NewParser()
			st, err := p.Parse(lex)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%#v", st)
		})
	}
}

// func TestLexerBasic(t *testing.T) {
// 	l, err := lex.LexString("", `"this is a simple quoted string"`)
// 	if err != nil {
// 		t.Errorf("LexString: %v", err)
// 	}
// 	for {
// 		tok, err := l.Next()
// 		if err == io.EOF {
// 			return
// 		}
// 		if err != nil {
// 			t.Fatalf("Next: %v", err)
// 		}
// 		t.Logf("tok: %v", tok.GoString())
// 	}
// }
