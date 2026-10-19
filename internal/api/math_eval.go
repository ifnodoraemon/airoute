package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// maxMathExprLen bounds the input before tokenizing. Expressions longer than
	// this are never legitimate from an LLM tool call.
	maxMathExprLen = 512
	// maxMathDepth bounds recursion. Without it a payload of a few thousand '('
	// characters overflows the goroutine stack, which is a fatal (unrecoverable)
	// runtime error that kills the whole gateway process.
	maxMathDepth = 32
)

// evalSimpleMath evaluates clean arithmetic expressions (+, -, *, /, ^).
func evalSimpleMath(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}
	if len(expr) > maxMathExprLen {
		return 0, fmt.Errorf("expression too long (%d > %d bytes)", len(expr), maxMathExprLen)
	}
	p := &mathParser{input: expr}
	val, err := p.parseAddSub(0)
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected trailing input at position %d: %s", p.pos, p.input[p.pos:])
	}
	return val, nil
}

type mathParser struct {
	input string
	pos   int
}

func (p *mathParser) peek() (byte, bool) {
	if p.pos >= len(p.input) {
		return 0, false
	}
	return p.input[p.pos], true
}

func (p *mathParser) parseAddSub(depth int) (float64, error) {
	if depth > maxMathDepth {
		return 0, fmt.Errorf("expression nesting too deep (> %d)", maxMathDepth)
	}
	val, err := p.parseMulDiv(depth)
	if err != nil {
		return 0, err
	}
	for {
		op, ok := p.peek()
		if !ok || (op != '+' && op != '-') {
			return val, nil
		}
		p.pos++
		nextVal, err := p.parseMulDiv(depth)
		if err != nil {
			return 0, err
		}
		if op == '+' {
			val += nextVal
		} else {
			val -= nextVal
		}
	}
}

func (p *mathParser) parseMulDiv(depth int) (float64, error) {
	val, err := p.parsePower(depth)
	if err != nil {
		return 0, err
	}
	for {
		op, ok := p.peek()
		if !ok || (op != '*' && op != '/') {
			return val, nil
		}
		p.pos++
		nextVal, err := p.parsePower(depth)
		if err != nil {
			return 0, err
		}
		if op == '*' {
			val *= nextVal
		} else {
			if nextVal == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val /= nextVal
		}
	}
}

func (p *mathParser) parsePower(depth int) (float64, error) {
	if depth > maxMathDepth {
		return 0, fmt.Errorf("expression nesting too deep (> %d)", maxMathDepth)
	}
	val, err := p.parsePrimary(depth)
	if err != nil {
		return 0, err
	}
	if op, ok := p.peek(); ok && op == '^' {
		p.pos++
		exp, err := p.parsePower(depth + 1)
		if err != nil {
			return 0, err
		}
		val = math.Pow(val, exp)
	}
	return val, nil
}

func (p *mathParser) parsePrimary(depth int) (float64, error) {
	if depth > maxMathDepth {
		return 0, fmt.Errorf("expression nesting too deep (> %d)", maxMathDepth)
	}
	ch, ok := p.peek()
	if !ok {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if ch == '(' {
		p.pos++
		val, err := p.parseAddSub(depth + 1)
		if err != nil {
			return 0, err
		}
		if c, ok := p.peek(); !ok || c != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	if ch == '-' {
		p.pos++
		val, err := p.parsePrimary(depth + 1)
		return -val, err
	}

	i := p.pos
	for i < len(p.input) && ((p.input[i] >= '0' && p.input[i] <= '9') || p.input[i] == '.') {
		i++
	}
	if i == p.pos {
		return 0, fmt.Errorf("invalid token at position %d: %s", p.pos, p.input[p.pos:])
	}
	numStr := p.input[p.pos:i]
	p.pos = i
	return strconv.ParseFloat(numStr, 64)
}
