package tools

import (
	"errors"
	"strings"
	"time"
)

func ParseDuration(dur string) (time.Duration, error) {
	splitByDays := strings.Split(dur, "d")
	switch len(splitByDays) {
	case 1:
		return time.ParseDuration(dur)
	case 2:
		days, err := time.ParseDuration(splitByDays[0] + "h")
		if err != nil {
			return 0, errors.New("failed to parse " + dur)
		}

		var parsedDuration time.Duration = 0
		if splitByDays[1] != "" {
			parsedDuration, err = time.ParseDuration(splitByDays[1])
			if err != nil {
				return 0, errors.New("failed to parse " + dur)
			}
		}

		return days*24 + parsedDuration, nil
	default:
		return 0, errors.New("failed to parse " + dur)
	}
}
