package main

import "testing"

func TestSelectVersion(t *testing.T) {
	tests := []struct {
		name             string
		linkerVersion    string
		buildInfoVersion string
		expected         string
	}{
		{
			name:             "prefers linker version when injected",
			linkerVersion:    "0.12.2",
			buildInfoVersion: "(devel)",
			expected:         "0.12.2",
		},
		{
			name:             "falls back to build info version",
			linkerVersion:    "dev",
			buildInfoVersion: "v0.10.1-0.20260409014410-3ee80c67684d",
			expected:         "v0.10.1-0.20260409014410-3ee80c67684d",
		},
		{
			name:             "handles devel values from both sources",
			linkerVersion:    "(devel)",
			buildInfoVersion: "(devel)",
			expected:         "unknown",
		},
		{
			name:             "handles empty values from both sources",
			linkerVersion:    "",
			buildInfoVersion: "",
			expected:         "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectVersion(tt.linkerVersion, tt.buildInfoVersion); got != tt.expected {
				t.Fatalf("selectVersion() = %q, want %q", got, tt.expected)
			}
		})
	}
}
