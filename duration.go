package rwmutexplus

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ParseDuration parses a duration string, adding
// support for the "d" unit meaning number of days,
// where a day is assumed to be 24h. The maximum
// input string length is 1024.

func ParseDuration(s string) (time.Duration, error) {
	if len(s) > 1024 {
		return 0, fmt.Errorf("parsing duration: input string too long")
	}
	var inNumber bool
	var numStart int
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == 'd' {
			daysStr := s[numStart:i]
			days, err := strconv.ParseFloat(daysStr, 64)
			if err != nil {
				return 0, err
			}
			hours := days * 24.0
			hoursStr := strconv.FormatFloat(hours, 'f', -1, 64)
			s = s[:numStart] + hoursStr + "h" + s[i+1:]
			i--
			continue
		}
		if !inNumber {
			numStart = i
		}
		inNumber = (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == '+'
	}
	return time.ParseDuration(s)
}

// GetDurationEnvOrDefault returns the duration value of an environment variable or the default value if not set
func GetDurationEnvOrDefault(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if duration, err := ParseDuration(val); err == nil {
			return duration
		}
	}
	return defaultValue
}
