package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsInteractiveInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	// A buffered (non-*os.File) input is treated as non-interactive, so the prompt
	// reads from the same stream it gates on.
	assert.False(t, isInteractiveInput(cmd))
}

func TestIsAffirmative(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "y\n", want: true},
		{in: "Y", want: true},
		{in: " yes \n", want: true},
		{in: "YES", want: true},
		{in: "n\n", want: false},
		{in: "no", want: false},
		{in: "", want: false},
		{in: "maybe", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, isAffirmative(tt.in))
		})
	}
}
