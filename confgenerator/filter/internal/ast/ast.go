package ast

import (
	"fmt"
	"strconv"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
)

type Attrib interface{}

type Member []string

type Restriction struct {
	Operator string
	LHS      Member
	RHS      string
}

func NewRestriction(lhs, operator, rhs Attrib) (*Restriction, error) {
	var r Restriction
	switch operator := operator.(type) {
	case string:
		r.Operator = operator
	case *token.Token:
		r.Operator = string(operator.Lit)
	default:
		return nil, fmt.Errorf("unknown operator: %v", operator)
	}
	switch lhs := lhs.(type) {
	case Member:
		r.LHS = lhs
	default:
		return nil, fmt.Errorf("unknown lhs: %v", lhs)
	}
	switch rhs := rhs.(type) {
	case nil:
	case Member:
		// BNF parses values as Member, even if they are singular
		if len(rhs) != 1 {
			return nil, fmt.Errorf("unexpected rhs: %v", rhs)
		}
		r.RHS = rhs[0]
	default:
		return nil, fmt.Errorf("unknown rhs: %v", rhs)
	}
	return &r, nil
}

type Expression interface {
}

type Conjunction []Expression
type Disjunction []Expression

type Negation struct {
	Expression
}

func ParseText(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	// Add quotes
	str = `"` + str + `"`
	// TODO: Support all escape sequences
	return strconv.Unquote(str)
}
func ParseString(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	// TODO: Support all escape sequences
	return strconv.Unquote(str)
}
