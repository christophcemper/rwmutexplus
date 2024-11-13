package rwmutexplus

import (
	"os"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	const day = 24 * time.Hour
	for i, tc := range []struct {
		input  string
		expect time.Duration
	}{
		{
			input:  "3h",
			expect: 3 * time.Hour,
		},
		{
			input:  "1d",
			expect: day,
		},
		{
			input:  "1d30m",
			expect: day + 30*time.Minute,
		},
		{
			input:  "1m2d",
			expect: time.Minute + day*2,
		},
		{
			input:  "1m2d30s",
			expect: time.Minute + day*2 + 30*time.Second,
		},
		{
			input:  "1d2d",
			expect: 3 * day,
		},
		{
			input:  "1.5d",
			expect: time.Duration(1.5 * float64(day)),
		},
		{
			input:  "4m1.25d",
			expect: 4*time.Minute + time.Duration(1.25*float64(day)),
		},
		{
			input:  "-1.25d12h",
			expect: time.Duration(-1.25*float64(day)) - 12*time.Hour,
		},
	} {
		actual, err := ParseDuration(tc.input)
		if err != nil {
			t.Errorf("Test %d ('%s'): Got error: %v", i, tc.input, err)
			continue
		}
		if actual != tc.expect {
			t.Errorf("Test %d ('%s'): Expected=%s Actual=%s", i, tc.input, tc.expect, actual)
		}
	}
}

func TestGetDurationEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue time.Duration
		want         time.Duration
	}{
		{
			name:         "env not set returns default",
			key:          "TEST_DURATION_NOT_SET",
			envValue:     "",
			defaultValue: 5 * time.Second,
			want:         5 * time.Second,
		},
		{
			name:         "valid duration returns parsed value",
			key:          "TEST_DURATION_VALID",
			envValue:     "10s",
			defaultValue: 5 * time.Second,
			want:         10 * time.Second,
		},
		{
			name:         "invalid duration returns default",
			key:          "TEST_DURATION_INVALID",
			envValue:     "invalid",
			defaultValue: 5 * time.Second,
			want:         5 * time.Second,
		},
		{
			name:         "days duration parsed correctly",
			key:          "TEST_DURATION_DAYS",
			envValue:     "2d",
			defaultValue: 5 * time.Second,
			want:         48 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := GetDurationEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetDurationEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}
