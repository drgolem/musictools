package decoders

import (
	"fmt"
	"path/filepath"
	"strings"

	"learnRingbuffer/pkg/decoders/flac"
	"learnRingbuffer/pkg/decoders/mp3"
	"learnRingbuffer/pkg/decoders/wav"
	"learnRingbuffer/pkg/types"
)

// NewDecoder creates and opens the appropriate decoder based on file extension.
// Supports .mp3, .flac, .fla, and .wav formats.
// Returns an opened decoder ready for use, or an error if the format is unsupported
// or the file cannot be opened.
func NewDecoder(fileName string) (types.AudioDecoder, error) {
	ext := strings.ToLower(filepath.Ext(fileName))

	var decoder types.AudioDecoder

	switch ext {
	case ".mp3":
		decoder = mp3.NewDecoder()
	case ".flac", ".fla":
		decoder = flac.NewDecoder()
	case ".wav":
		decoder = wav.NewDecoder()
	default:
		return nil, fmt.Errorf("unsupported file format: %s (supported: .mp3, .flac, .fla, .wav)", ext)
	}

	if err := decoder.Open(fileName); err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", fileName, err)
	}

	return decoder, nil
}
