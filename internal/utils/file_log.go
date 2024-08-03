package utils

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteToJSONLFile appends a JSON-encoded representation of the given data to the specified file.
// If the file does not exist, it will be created.
//
// Parameters:
//
//	file - The path to the file where the data should be written.
//	data - The data to be JSON-encoded and written to the file.
//
// Returns:
//
//	An error if the file could not be created or written to, otherwise nil.
func WriteToJSONLFile(file string, data interface{}) error {
	// Open file for writing
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	// Create JSON encoder
	enc := json.NewEncoder(f)

	// Write data to file
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write data to file: %w", err)
	}

	return nil
}
