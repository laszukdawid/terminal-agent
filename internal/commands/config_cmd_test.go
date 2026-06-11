package commands

import (
	"testing"
	"time"
)

func TestBedrockPriceCacheExpired(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		lastChecked string
		want        bool
	}{
		{name: "fresh", lastChecked: now.Add(-23 * time.Hour).Format(time.RFC3339), want: false},
		{name: "exact ttl", lastChecked: now.Add(-24 * time.Hour).Format(time.RFC3339), want: false},
		{name: "expired", lastChecked: now.Add(-25 * time.Hour).Format(time.RFC3339), want: true},
		{name: "invalid", lastChecked: "not-a-time", want: true},
		{name: "empty", lastChecked: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bedrockPriceCacheExpired(tt.lastChecked, now)
			if got != tt.want {
				t.Fatalf("expired = %v, want %v", got, tt.want)
			}
		})
	}
}
