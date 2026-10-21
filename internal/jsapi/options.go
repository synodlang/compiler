package jsapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/scene"
)

// maxSeed is the largest seed (2^53): above it a number is not exact.
const maxSeed = 1 << 53

// decodeOptions reads the options argument into dst, a struct of
// json.RawMessage fields. Unknown options are rejected.
func decodeOptions(raw string, dst any) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "{") {
		return errors.New("options must be an object")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		msg := err.Error()
		if rest, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
			return fmt.Errorf("unknown option %s", rest)
		}
		return fmt.Errorf("options are not valid: %v", err)
	}
	if err := noTrailing(dec); err != nil {
		return fmt.Errorf("options are not valid: %v", err)
	}
	return nil
}

// present reports whether an option was given (and is not null).
func present(r json.RawMessage) bool {
	return len(r) > 0 && !bytes.Equal(bytes.TrimSpace(r), []byte("null"))
}

func optString(name string, r json.RawMessage) (string, error) {
	var s string
	if len(r) == 0 || r[0] != '"' || json.Unmarshal(r, &s) != nil {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return s, nil
}

func optBool(name string, r json.RawMessage) (bool, error) {
	switch strings.TrimSpace(string(r)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func optStrings(name string, r json.RawMessage) ([]string, error) {
	var out []string
	if len(r) == 0 || r[0] != '[' || json.Unmarshal(r, &out) != nil {
		return nil, fmt.Errorf("%s must be an array of strings", name)
	}
	return out, nil
}

// optNumber returns the literal text of a JSON number option; ok is false
// for any other value, whose text is returned for the message.
func optNumber(r json.RawMessage) (text string, ok bool) {
	t := strings.TrimSpace(string(r))
	if len(t) > 0 && t[0] == '"' {
		var s string
		if json.Unmarshal(r, &s) == nil {
			return s, false
		}
	}
	dec := json.NewDecoder(strings.NewReader(t))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) == nil {
		if num, isNum := v.(json.Number); isNum {
			return num.String(), true
		}
	}
	return t, false
}

// parseStyle reads the style option: rich unless "short".
func parseStyle(r json.RawMessage) (rich bool, err error) {
	if !present(r) {
		return true, nil
	}
	s, err := optString("style", r)
	if err != nil {
		return false, err
	}
	switch s {
	case "rich":
		return true, nil
	case "short":
		return false, nil
	}
	return false, fmt.Errorf("style needs rich or short, not %q", s)
}

// parseSeed applies the command line's seed rule.
func parseSeed(r json.RawMessage) (*uint64, error) {
	if !present(r) {
		return nil, nil
	}
	text, ok := optNumber(r)
	n, err := strconv.ParseUint(text, 10, 64)
	if !ok || err != nil || n > maxSeed {
		return nil, fmt.Errorf("seed needs a whole number from 0 to %d, not %q", uint64(maxSeed), text)
	}
	return &n, nil
}

// parsePace applies the command line's pace rule; the result is in
// thousandths.
func parsePace(r json.RawMessage) (int, error) {
	if !present(r) {
		return 1000, nil
	}
	text, ok := optNumber(r)
	n, good := pipeline.ParsePace(text)
	if !ok || !good {
		return 0, fmt.Errorf("pace needs a number from 0.25 to 4, not %q", text)
	}
	return n, nil
}

func parseMode(r json.RawMessage) (scene.Mode, error) {
	if !present(r) {
		return scene.Ordered, nil
	}
	s, err := optString("mode", r)
	if err != nil {
		return 0, err
	}
	switch s {
	case "ordered":
		return scene.Ordered, nil
	case "proportional":
		return scene.Proportional, nil
	}
	return 0, fmt.Errorf("mode needs ordered or proportional, not %q", s)
}

func parsePaths(r json.RawMessage) ([]string, error) {
	if !present(r) {
		return nil, nil
	}
	return optStrings("paths", r)
}
