// Package period parses durations written as a count and a unit: 36h, 1d, 2w.
//
// time.ParseDuration is not used because it has no day or week unit, and
// its "m" means minutes, which an agent writing "1m" for one month would not
// expect. Here "m" is rejected outright.
package period

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	Hour = time.Hour
	Day  = 24 * Hour
	Week = 7 * Day
)

// Parse reads "<count><unit>" where unit is h, d, or w.
func Parse(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("%q: want a count and a unit, such as 36h, 1d, or 2w", s)
	}
	digits, unit := s[:len(s)-1], s[len(s)-1:]
	var per time.Duration
	switch unit {
	case "h":
		per = Hour
	case "d":
		per = Day
	case "w":
		per = Week
	case "m":
		return 0, fmt.Errorf("%q: m could mean minutes or months; use h, d, or w", s)
	default:
		return 0, fmt.Errorf("%q: unknown unit %q; use h, d, or w", s, unit)
	}
	if strings.HasPrefix(digits, "-") || strings.HasPrefix(digits, "+") {
		return 0, fmt.Errorf("%q: want a count and a unit, such as 36h, 1d, or 2w", s)
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n > int64(math.MaxInt64/per) {
		return 0, fmt.Errorf("%q: want a count and a unit, such as 36h, 1d, or 2w", s)
	}
	return time.Duration(n) * per, nil
}
