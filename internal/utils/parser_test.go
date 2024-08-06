package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindCodeTag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *CodeObject
		wantErr bool
	}{
		{
			name:    "No code tags",
			input:   "This is a test string without code tags.",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Opening code tag only",
			input:   "This is a test string with <code> tag only.",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Closing code tag only",
			input:   "This is a test string with </code> tag only.",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Valid code tags with code and lang",
			input:   "This is a test string with {\"code\": \"valid code\", \"lang\": \"bash\"} tags.",
			want:    &CodeObject{Code: "valid code", Lang: "bash"},
			wantErr: false,
		},
		{
			name:    "Valid code tags with text with only code",
			input:   "This is a test string with {\"code\": \"valid code\"} tags.",
			want:    &CodeObject{Code: "valid code", Lang: ""},
			wantErr: false,
		},
		{
			name:    "Valid code tags without text",
			input:   "This is a test string with {\"code\": \"\"} tags.",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindCodeObject(&tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindCodeTag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.want != nil {
				assert.NotNil(t, got)
				assert.Equal(t, got.Code, tt.want.Code)
				assert.Equal(t, got.Lang, tt.want.Lang)
			} else {
				assert.Nil(t, got)
			}

		})
	}
}
