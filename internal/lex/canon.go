package lex

import "strings"

// The canonical spellings of numbers, durations and colors (spec/01 sections
// 1.5 and 1.7, spec/10). All functions work on the decimal text and never go
// through floating point, so no value is ever rounded.

// splitNumber splits a number into sign, integer digits and fraction digits.
func splitNumber(s string) (neg bool, intPart, frac string) {
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intPart, frac, _ = strings.Cut(s, ".")
	return neg, intPart, frac
}

// joinNumber assembles a canonical number: no leading zeros (one is kept),
// no trailing fraction zeros, and zero without a sign.
func joinNumber(neg bool, intPart, frac string) string {
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	frac = strings.TrimRight(frac, "0")
	if intPart == "0" && frac == "" {
		neg = false
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(intPart)
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}
	return b.String()
}

// CanonicalNumber returns the canonical spelling of the number s:
// "0.50" is "0.5", "1.0" is "1", "007" is "7" and "-0.0" is "0".
func CanonicalNumber(s string) string {
	neg, i, f := splitNumber(s)
	return joinNumber(neg, i, f)
}

// CanonicalDuration returns the canonical spelling of the duration s, which
// ends in "ms" or "s". Whole millisecond values that are a multiple of 1000
// are written in seconds ("1000ms" is "1s", "0ms" is "0s"); everything
// else, including fractions, is written in milliseconds ("1.5s" is
// "1500ms", "0.25s" is "250ms").
func CanonicalDuration(s string) string {
	num, seconds := strings.TrimSuffix(s, "ms"), false
	if num == s {
		num, seconds = strings.TrimSuffix(s, "s"), true
	}
	neg, i, f := splitNumber(num)
	if seconds {
		// Shift the decimal point three places right.
		for len(f) < 3 {
			f += "0"
		}
		i, f = i+f[:3], f[3:]
	}
	ms := joinNumber(neg, i, f)
	if ms == "0" {
		return "0s"
	}
	if !strings.Contains(ms, ".") && strings.HasSuffix(ms, "000") {
		return strings.TrimSuffix(ms, "000") + "s"
	}
	return ms + "ms"
}

// CanonicalColor returns s with its hex digits in upper case.
func CanonicalColor(s string) string {
	return strings.ToUpper(s)
}
