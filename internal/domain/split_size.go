package domain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

var minimumSplitSizePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(MB|GB|%)$`)

// ParseMinimumSplitSize resolves an absolute or percentage split threshold in bytes.
func ParseMinimumSplitSize(input string, usableCapacity int64) (int64, error) {
	if usableCapacity <= 0 {
		return 0, fmt.Errorf("usable capacity must be positive")
	}
	matches := minimumSplitSizePattern.FindStringSubmatch(input)
	if matches == nil {
		return 0, fmt.Errorf("invalid minimum split size %q", input)
	}
	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse minimum split size %q: %w", input, err)
	}

	if matches[2] == "%" {
		if value > 100 {
			return 0, fmt.Errorf("minimum split percentage %q exceeds 100%%", input)
		}
		return usableCapacity/100*value + (usableCapacity%100*value+99)/100, nil
	}

	multiplier := int64(1_000_000)
	if matches[2] == "GB" {
		multiplier = 1_000_000_000
	}
	if value > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("minimum split size %q exceeds supported byte range", input)
	}
	resolved := value * multiplier
	if resolved > usableCapacity {
		return 0, fmt.Errorf("minimum split size %q exceeds usable capacity", input)
	}
	return resolved, nil
}
