package utils

import (
	"fmt"
	"strings"
)

// FindCodeTag finds the <code> tag in the input
// and returns the content of the tag.
// If the tag is not found, it returns an error.
func FindCodeTag(input *string) (string, error) {
	// Find the <code> tag in the input
	codeTag := "<code>"
	codeTagEnd := "</code>"

	start := strings.Index(*input, codeTag)
	if start == -1 {
		return "", fmt.Errorf("no <code> tag found in the input")
	}

	start += len(codeTag)
	end := strings.Index((*input)[start:], codeTagEnd)
	if end == -1 {
		return "", fmt.Errorf("no </code> tag found in the input")
	}

	return (*input)[start : start+end], nil
}
