package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// evalSimpleMath evaluates clean arithmetic expressions (+, -, *, /, ^).
func evalSimpleMath(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}
	return parseAddSub(&expr)
}

func parseAddSub(s *string) (float64, error) {
	val, err := parseMulDiv(s)
	if err != nil {
		return 0, err
	}
	for len(*s) > 0 {
		op := (*s)[0]
		if op != '+' && op != '-' {
			break
		}
		*s = (*s)[1:]
		nextVal, err := parseMulDiv(s)
		if err != nil {
			return 0, err
		}
		if op == '+' {
			val += nextVal
		} else {
			val -= nextVal
		}
	}
	return val, nil
}

func parseMulDiv(s *string) (float64, error) {
	val, err := parsePower(s)
	if err != nil {
		return 0, err
	}
	for len(*s) > 0 {
		op := (*s)[0]
		if op != '*' && op != '/' {
			break
		}
		*s = (*s)[1:]
		nextVal, err := parsePower(s)
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
	return val, nil
}

func parsePower(s *string) (float64, error) {
	val, err := parsePrimary(s)
	if err != nil {
		return 0, err
	}
	if len(*s) > 0 && (*s)[0] == '^' {
		*s = (*s)[1:]
		exp, err := parsePower(s)
		if err != nil {
			return 0, err
		}
		val = math.Pow(val, exp)
	}
	return val, nil
}

func parsePrimary(s *string) (float64, error) {
	if len(*s) == 0 {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if (*s)[0] == '(' {
		*s = (*s)[1:]
		val, err := parseAddSub(s)
		if err != nil {
			return 0, err
		}
		if len(*s) == 0 || (*s)[0] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		*s = (*s)[1:]
		return val, nil
	}
	if (*s)[0] == '-' {
		*s = (*s)[1:]
		val, err := parsePrimary(s)
		return -val, err
	}

	i := 0
	for i < len(*s) && (((*s)[i] >= '0' && (*s)[i] <= '9') || (*s)[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("invalid token: %s", *s)
	}
	numStr := (*s)[:i]
	*s = (*s)[i:]
	return strconv.ParseFloat(numStr, 64)
}
