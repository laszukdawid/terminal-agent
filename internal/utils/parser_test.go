package utils

import (
	"testing"
)

func TestFindCodeTag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "No code tags",
			input:   "This is a test string without code tags.",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Opening code tag only",
			input:   "This is a test string with <code> tag only.",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Closing code tag only",
			input:   "This is a test string with </code> tag only.",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Valid code tags with text",
			input:   "This is a test string with <code>valid code</code> tags.",
			want:    "valid code",
			wantErr: false,
		},
		{
			name:    "Valid code tags without text",
			input:   "This is a test string with <code></code> tags.",
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindCodeTag(&tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindCodeTag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FindCodeTag() = %v, want %v", got, tt.want)
			}
		})
	}
}
