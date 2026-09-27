package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Grades travel as JSON numbers but are never decoded into a float: a float64
// cannot tell 9.95 from 9.949999…, and deciding "one decimal too many" on that
// representation is a bug waiting for the right grade. The edge keeps the
// number's exact text (json.RawMessage) and hands it on as a string; the
// service that owns the scale judges it with decimal arithmetic.

// hundredthsPerUnit is the fixed-point unit every grade and scale uses.
const hundredthsPerUnit = 100

// numberText returns the exact text of raw when it is a JSON number token, and
// false for anything else — a string ("9.5"), null, true, an object, or a
// missing field (empty raw).
func numberText(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !(raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9')) {
		return "", false
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return "", false
	}
	return n.String(), true
}

// maxDecimalText caps a mark's text: no mark needs more than 16 characters,
// and the bound keeps big.Rat from ever doing unbounded work.
const maxDecimalText = 16

// plainDecimal is the only shape a mark may take: optional minus, digits,
// optional fraction. Exponent ("1e9"), hex and binary forms are refused BEFORE
// big.Rat sees them — SetString would build "1e1000000000" digit by digit.
var plainDecimal = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// parseHundredths converts a JSON number with at most two decimals into exact
// hundredths (5 → 500, 69.5 → 6950). Only plain decimals of at most 16
// characters are read.
func parseHundredths(raw json.RawMessage) (int, bool) {
	text, ok := numberText(raw)
	if !ok || len(text) > maxDecimalText || !plainDecimal.MatchString(text) {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return 0, false
	}
	scaled := new(big.Rat).Mul(r, big.NewRat(hundredthsPerUnit, 1))
	if !scaled.IsInt() || !scaled.Num().IsInt64() {
		return 0, false
	}
	return int(scaled.Num().Int64()), true
}

// hundredthsNumber prints hundredths as the shortest exact JSON number
// (500 → 5, 950 → 9.5, 925 → 9.25).
func hundredthsNumber(h int) json.Number {
	sign := ""
	if h < 0 {
		sign, h = "-", -h
	}
	whole, frac := h/hundredthsPerUnit, h%hundredthsPerUnit
	if frac == 0 {
		return json.Number(sign + strconv.Itoa(whole))
	}
	return json.Number(sign + strings.TrimRight(fmt.Sprintf("%d.%02d", whole, frac), "0"))
}
