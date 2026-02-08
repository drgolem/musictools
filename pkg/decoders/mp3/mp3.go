package mp3

import (
	"fmt"

	"github.com/drgolem/go-mpg123/mpg123"
)

// Decoder wraps the mpg123.Decoder to provide MP3 decoding capabilities.
// Implements types.AudioDecoder interface.
type Decoder struct {
	decoder  *mpg123.Decoder
	rate     int
	channels int
	encoding int
}

// NewDecoder creates a new MP3 decoder
func NewDecoder() *Decoder {
	return &Decoder{}
}

// GetFormat returns the audio format (rate, channels, encoding)
func (d *Decoder) GetFormat() (int, int, int) {
	return d.rate, d.channels, d.encoding
}

// DecodeSamples decodes the specified number of samples into the audio buffer
// Returns the number of samples decoded (not bytes)
func (d *Decoder) DecodeSamples(samples int, audio []byte) (int, error) {
	if d.decoder == nil {
		return 0, fmt.Errorf("decoder not initialized")
	}

	// Use mpg123's DecodeSamples which correctly handles all audio formats
	// (mono/stereo, 16/24/32-bit)
	return d.decoder.DecodeSamples(samples, audio)
}

// Open opens and initializes an MP3 file for decoding
func (d *Decoder) Open(fileName string) error {
	// Create new decoder
	decoder, err := mpg123.NewDecoder("")
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	// Open the file
	err = decoder.Open(fileName)
	if err != nil {
		decoder.Delete()
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}

	// Get audio format
	rate, channels, encoding := decoder.GetFormat()

	d.decoder = decoder
	d.rate = rate
	d.channels = channels
	d.encoding = encoding

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

// Encoding returns the encoding format
func (d *Decoder) Encoding() int {
	return d.encoding
}
