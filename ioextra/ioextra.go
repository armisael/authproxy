package ioextra

import (
	"bytes"
)

type ClosingReader struct {
	bytes.Reader
}

func (rnc ClosingReader) Close() error {
	return nil
}

func NewBufferizedClosingReader(b []byte) *ClosingReader {
	return &ClosingReader{*bytes.NewReader(b)}
}
