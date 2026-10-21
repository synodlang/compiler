package lex

import "testing"

func TestCanonicalNumber(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0", "0"}, {"0.50", "0.5"}, {"1.0", "1"}, {"007", "7"}, {"-0.0", "0"},
		{"-1", "-1"}, {"-0", "0"}, {"000", "0"}, {"10", "10"}, {"100.100", "100.1"},
		{"-007.500", "-7.5"}, {"0.05", "0.05"}, {"3.14159", "3.14159"},
		{"12345678901234567890.123456789012345678900", "12345678901234567890.1234567890123456789"},
	}
	for _, tc := range tests {
		if got := CanonicalNumber(tc.in); got != tc.want {
			t.Errorf("CanonicalNumber(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCanonicalDuration(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0s", "0s"}, {"0ms", "0s"}, {"-0s", "0s"}, {"0.0s", "0s"},
		{"1500ms", "1500ms"}, {"1.5s", "1500ms"}, {"1000ms", "1s"}, {"2000ms", "2s"},
		{"0.25s", "250ms"}, {"60s", "60s"}, {"-1000ms", "-1s"}, {"0.5ms", "0.5ms"},
		{"0.50ms", "0.5ms"}, {"1s", "1s"}, {"5ms", "5ms"}, {"01s", "1s"},
		{"100000ms", "100s"}, {"1.2345s", "1234.5ms"}, {"0.001s", "1ms"},
		{"1.0s", "1s"}, {"10ms", "10ms"}, {"-1.5s", "-1500ms"}, {"0.0001s", "0.1ms"},
		{"2.000s", "2s"},
	}
	for _, tc := range tests {
		if got := CanonicalDuration(tc.in); got != tc.want {
			t.Errorf("CanonicalDuration(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCanonicalColor(t *testing.T) {
	tests := []struct{ in, want string }{
		{"#F4A340", "#F4A340"}, {"#deface", "#DEFACE"}, {"#aBc012", "#ABC012"},
	}
	for _, tc := range tests {
		if got := CanonicalColor(tc.in); got != tc.want {
			t.Errorf("CanonicalColor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCanonicalIdempotent checks that canonical spellings are fixed points.
func TestCanonicalIdempotent(t *testing.T) {
	for _, s := range []string{"0.50", "-0.0", "007", "1.5", "100", "0.001"} {
		c := CanonicalNumber(s)
		if CanonicalNumber(c) != c {
			t.Errorf("CanonicalNumber not idempotent on %q", s)
		}
	}
	for _, s := range []string{"1500ms", "1.5s", "0ms", "1000ms", "0.5ms", "0.0001s"} {
		c := CanonicalDuration(s)
		if CanonicalDuration(c) != c {
			t.Errorf("CanonicalDuration not idempotent on %q (%q)", s, c)
		}
	}
}
