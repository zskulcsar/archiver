package domain

import (
	"fmt"
	"regexp"
	"strconv"
)

var lossTolerancePattern = regexp.MustCompile(`^([0-9]|[1-4][0-9]|50)%$`)

// ParseLossTolerance parses the local PAR2 loss-tolerance percentage.
func ParseLossTolerance(input string) (int, error) {
	matches := lossTolerancePattern.FindStringSubmatch(input)
	if matches == nil {
		return 0, fmt.Errorf("invalid loss tolerance %q: use a whole percentage from 0%% through 50%%", input)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("parse loss tolerance %q: %w", input, err)
	}

	return value, nil
}
