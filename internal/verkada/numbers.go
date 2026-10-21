package verkada

import (
	"encoding/json"
	"strconv"
	"strings"
)

// parseNumber reads a JSON value as a number. Verkada sends some numbers as JSON
// numbers (20.8), some as strings ("1700000000") and some flags as booleans (true).
// It returns false for null, empty or text values.
func parseNumber(raw json.RawMessage) (float64, bool) {
	text := strings.Trim(string(raw), `"`)

	switch text {
	case "true":
		return 1, true
	case "false":
		return 0, true
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}

	return value, true
}
