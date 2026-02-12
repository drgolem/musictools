package flac

import (
	"fmt"

	goflac "github.com/drgolem/go-flac/flac"
)

// Decoder wraps the go-flac decoder to provide FLAC decoding capabilities.
// Implements types.AudioDecoder interface.
type Decoder struct {
	decoder  *goflac.FlacDecoder
	rate     int
	channels int
	bps      int // bits per sample
}

// NewDecoder creates a new FLAC decoder
// Uses 16-bit output by default
func NewDecoder() *Decoder {
	return &Decoder{}
}

// GetFormat returns the audio format (rate, channels, bits per sample)
func (d *Decoder) GetFormat() (int, int, int) {
	return d.rate, d.channels, d.bps
}

// DecodeSamples decodes the specified number of samples into the audio buffer
func (d *Decoder) DecodeSamples(samples int, audio []byte) (int, error) {
	if d.decoder == nil {
		return 0, fmt.Errorf("decoder not initialized")
	}

	// Decode PCM data from FLAC
	n, err := d.decoder.DecodeSamples(samples, audio)
	return n, err
}

// Open opens and initializes a FLAC file for decoding
func (d *Decoder) Open(fileName string) error {
	// Create new decoder with 16-bit output by default
	// This can be adjusted to 24 or 32 if needed
	decoder, err := goflac.NewFlacFrameDecoder(16)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	// Open the FLAC file
	err = decoder.Open(fileName)
	if err != nil {
		decoder.Delete()
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}

	// Get audio format
	rate, channels, bps := decoder.GetFormat()

	d.decoder = decoder
	d.rate = rate
	d.channels = channels
	d.bps = bps

	return nil
}

// Close closes the decoder and releases resources
func (d *Decoder) Close() error {
	if d.decoder != nil {
		d.decoder.Close()
		d.decoder.Delete()
		d.decoder = nil
	}
	return nil
}

// Rate returns the sample rate in Hz
func (d *Decoder) Rate() int {
	return d.rate
}

// Channels returns the number of audio channels
func (d *Decoder) Channels() int {
	return d.channels
}

// Encoding returns the bits per sample (for consistency with MP3 decoder)
func (d *Decoder) Encoding() int {
	return d.bps
}

// BitsPerSample returns the bits per sample
func (d *Decoder) BitsPerSample() int {
	return d.bps
}
