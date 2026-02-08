package wav

import (
	"fmt"
	"os"

	"github.com/youpy/go-wav"
)

// Decoder wraps go-wav for decoding WAV audio files.
// Implements types.AudioDecoder interface.
type Decoder struct {
	file     *os.File
	reader   *wav.Reader
	rate     int
	channels int
	bps      int
	format   uint16
}

// NewDecoder creates a new WAV decoder
func NewDecoder() *Decoder {
	return &Decoder{}
}

// Open opens a WAV file for decoding
func (d *Decoder) Open(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open WAV file: %w", err)
	}

	reader := wav.NewReader(file)
	format, err := reader.Format()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to read WAV format: %w", err)
	}

	// Validate format
	if format.AudioFormat != wav.AudioFormatPCM {
		file.Close()
		return fmt.Errorf("unsupported WAV format: %d (only PCM supported)", format.AudioFormat)
	}

	d.file = file
	d.reader = reader
	d.rate = int(format.SampleRate)
	d.channels = int(format.NumChannels)
	d.bps = int(format.BitsPerSample)
	d.format = format.AudioFormat

	return nil
}

// Close closes the WAV file
func (d *Decoder) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// GetFormat returns the audio format (sample rate, channels, bits per sample)
func (d *Decoder) GetFormat() (rate, channels, bitsPerSample int) {
	return d.rate, d.channels, d.bps
}

// DecodeSamples decodes up to 'samples' audio samples into the provided buffer
//
// Parameters:
//   - samples: number of samples to decode (not bytes)
//   - audio: buffer to write decoded audio data
//
// Returns:
//   - number of samples actually decoded
//   - error if any
//
// The buffer must be large enough to hold: samples * channels * (bitsPerSample/8) bytes
//
// Example:
//
//	decoder.DecodeSamples(1024, buffer)  // Decode 1024 samples
//	bytesWritten := samplesRead * channels * (bitsPerSample/8)
func (d *Decoder) DecodeSamples(samples int, audio []byte) (int, error) {
	if d.reader == nil {
		return 0, fmt.Errorf("decoder not initialized")
	}

	bytesPerSample := d.bps / 8
	totalSamples := 0

	// Read samples one at a time (go-wav reads sample by sample)
	for i := 0; i < samples; i++ {
		samplesData, err := d.reader.ReadSamples(1)
		if err != nil {
			// End of file or error
			return totalSamples, err
		}

		if len(samplesData) == 0 {
			// No more data
			return totalSamples, nil
		}

		// Convert samples to bytes and write to buffer
		// go-wav returns samples as []wav.Sample which contains IntValue for each channel
		for ch := 0; ch < d.channels; ch++ {
			if ch >= len(samplesData[0].Values) {
				break
			}

			value := samplesData[0].Values[ch]
			offset := (totalSamples*d.channels + ch) * bytesPerSample

			// Check buffer bounds
			if offset+bytesPerSample > len(audio) {
				return totalSamples, nil
			}

			// Write sample bytes (little-endian)
			switch d.bps {
			case 8:
				audio[offset] = byte(value)
			case 16:
				audio[offset] = byte(value & 0xFF)
				audio[offset+1] = byte((value >> 8) & 0xFF)
			case 24:
				audio[offset] = byte(value & 0xFF)
				audio[offset+1] = byte((value >> 8) & 0xFF)
				audio[offset+2] = byte((value >> 16) & 0xFF)
			case 32:
				audio[offset] = byte(value & 0xFF)
				audio[offset+1] = byte((value >> 8) & 0xFF)
				audio[offset+2] = byte((value >> 16) & 0xFF)
				audio[offset+3] = byte((value >> 24) & 0xFF)
			default:
				return totalSamples, fmt.Errorf("unsupported bits per sample: %d", d.bps)
			}
		}

		totalSamples++
	}

	return totalSamples, nil
}
