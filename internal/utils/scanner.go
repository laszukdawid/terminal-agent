package utils

import (
	"bufio"
	"io"
)

// JSONLScannerMaxTokenSize is the maximum size of a single line the JSONL log
// scanners will read. bufio.Scanner defaults to a 64 KB token limit, which is
// too small for large logged queries, answers, or memory entries and surfaces
// as a "bufio.Scanner: token too long" error that aborts the whole read. Raise
// it to 64 MB, matching the session-log reader.
const JSONLScannerMaxTokenSize = 64 * 1024 * 1024

// jsonlScannerInitialBufferSize is the initial capacity of the scanner buffer;
// it grows up to JSONLScannerMaxTokenSize as needed.
const jsonlScannerInitialBufferSize = 64 * 1024

// NewJSONLScanner returns a bufio.Scanner for reading newline-delimited JSON
// records with a raised token-size limit, so that entries larger than the
// default 64 KB do not fail the read.
func NewJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, jsonlScannerInitialBufferSize), JSONLScannerMaxTokenSize)
	return scanner
}
