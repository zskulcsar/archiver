package domain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

var capacityPattern = regexp.MustCompile(`^([1-9][0-9]*)(MB|GB)$`)

// ParseCapacity converts a strict decimal MB or GB capacity value to bytes.
func ParseCapacity(input string) (int64, error) {
	matches := capacityPattern.FindStringSubmatch(input)
	if matches == nil {
		return 0, fmt.Errorf("invalid capacity %q: use a positive whole-number MB or GB value", input)
	}

	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse capacity %q: %w", input, err)
	}

	multiplier := int64(1_000_000)
	if matches[2] == "GB" {
		multiplier = 1_000_000_000
	}
	if value > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("capacity %q exceeds supported byte range", input)
	}

	return value * multiplier, nil
}
