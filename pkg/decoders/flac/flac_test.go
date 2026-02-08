package flac

import (
	"testing"
)

func TestNewDecoder(t *testing.T) {
	decoder := NewDecoder()
	if decoder == nil {
		t.Fatal("NewDecoder returned nil")
	}
}

func TestDecoderGetFormat(t *testing.T) {
	decoder := NewDecoder()

	// Before opening a file, format should be zero values
	rate, channels, encoding := decoder.GetFormat()
	if rate != 0 || channels != 0 || encoding != 0 {
		t.Errorf("Expected zero values before Open, got rate=%d, channels=%d, encoding=%d",
			rate, channels, encoding)
	}
}

func TestDecoderHelperMethods(t *testing.T) {
	decoder := NewDecoder()

	if decoder.Rate() != 0 {
		t.Errorf("Expected Rate() = 0, got %d", decoder.Rate())
	}
	if decoder.Channels() != 0 {
		t.Errorf("Expected Channels() = 0, got %d", decoder.Channels())
	}
	if decoder.Encoding() != 0 {
		t.Errorf("Expected Encoding() = 0, got %d", decoder.Encoding())
	}
	if decoder.BitsPerSample() != 0 {
		t.Errorf("Expected BitsPerSample() = 0, got %d", decoder.BitsPerSample())
	}
}

func TestDecoderClose(t *testing.T) {
	decoder := NewDecoder()

	// Should be safe to close without opening
	err := decoder.Close()
	if err != nil {
		t.Errorf("Close on unopened decoder failed: %v", err)
	}

	// Should be safe to close multiple times
	err = decoder.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestDecodeSamplesWithoutOpen(t *testing.T) {
	decoder := NewDecoder()

	buffer := make([]byte, 1024)
	_, err := decoder.DecodeSamples(len(buffer), buffer)
	if err == nil {
		t.Error("Expected error when decoding without opening file")
	}
}

// Example demonstrates basic usage of the FLAC decoder
func ExampleDecoder() {
	// Create a new decoder
	decoder := NewDecoder()
	defer decoder.Close()

	// Note: This example would require an actual FLAC file
	// err := decoder.Open("test.flac")
	// if err != nil {
	//     log.Fatal(err)
	// }

	// Get audio format
	// rate, channels, bps := decoder.GetFormat()
	// fmt.Printf("Format: %d Hz, %d channels, %d bits per sample\n", rate, channels, bps)

	// Decode audio samples
	// buffer := make([]byte, 4096)
	// n, err := decoder.DecodeSamples(len(buffer), buffer)
	// Process buffer[:n]
}
