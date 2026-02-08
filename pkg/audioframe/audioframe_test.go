package audioframe

import (
	"bytes"
	"testing"
)

func TestAudioFrameMarshalUnmarshal(t *testing.T) {
	// Create test audio frame
	original := AudioFrame{
		Format: FrameFormat{
			SampleRate:    44100,
			Channels:      2,
			BitsPerSample: 16,
		},
		Audio:        []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		SamplesCount: 4,
	}

	// Marshal to bytes
	data := original.Marshal()

	// Verify size
	expectedSize := 12 + len(original.Audio) // 12 byte header + audio data
	if len(data) != expectedSize {
		t.Errorf("Marshal size: got %d, want %d", len(data), expectedSize)
	}

	// Unmarshal back
	var decoded AudioFrame
	err := decoded.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify all fields
	if decoded.Format.SampleRate != original.Format.SampleRate {
		t.Errorf("SampleRate: got %d, want %d", decoded.Format.SampleRate, original.Format.SampleRate)
	}
	if decoded.Format.Channels != original.Format.Channels {
		t.Errorf("Channels: got %d, want %d", decoded.Format.Channels, original.Format.Channels)
	}
	if decoded.Format.BitsPerSample != original.Format.BitsPerSample {
		t.Errorf("BitsPerSample: got %d, want %d", decoded.Format.BitsPerSample, original.Format.BitsPerSample)
	}
	if decoded.SamplesCount != original.SamplesCount {
		t.Errorf("SamplesCount: got %d, want %d", decoded.SamplesCount, original.SamplesCount)
	}
	if !bytes.Equal(decoded.Audio, original.Audio) {
		t.Errorf("Audio data mismatch: got %v, want %v", decoded.Audio, original.Audio)
	}
}

func TestAudioFrameEmptyAudio(t *testing.T) {
	// Test with empty audio data
	original := AudioFrame{
		Format: FrameFormat{
			SampleRate:    48000,
			Channels:      1,
			BitsPerSample: 24,
		},
		Audio:        []byte{},
		SamplesCount: 0,
	}

	data := original.Marshal()

	var decoded AudioFrame
	err := decoded.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Audio) != 0 {
		t.Errorf("Audio length: got %d, want 0", len(decoded.Audio))
	}
}

func TestAudioFrameLargeData(t *testing.T) {
	// Test with large audio data
	largeAudio := make([]byte, 100000)
	for i := range largeAudio {
		largeAudio[i] = byte(i % 256)
	}

	original := AudioFrame{
		Format: FrameFormat{
			SampleRate:    96000,
			Channels:      8,
			BitsPerSample: 32,
		},
		Audio:        largeAudio,
		SamplesCount: 12500,
	}

	data := original.Marshal()

	var decoded AudioFrame
	err := decoded.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(decoded.Audio, original.Audio) {
		t.Error("Large audio data mismatch")
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		err  string
	}{
		{
			name: "empty buffer",
			data: []byte{},
			err:  "buffer too small",
		},
		{
			name: "incomplete header",
			data: make([]byte, 10),
			err:  "buffer too small",
		},
		{
			name: "audio length exceeds buffer",
			data: func() []byte {
				// Create header claiming 1000 bytes of audio but only provide header
				buf := make([]byte, 12)
				// Set audio length to 1000 at offset 8-12 (uint32, little-endian)
				buf[8] = 0xE8  // 1000 & 0xFF (232)
				buf[9] = 0x03  // (1000 >> 8) & 0xFF (3)
				buf[10] = 0x00 // (1000 >> 16) & 0xFF (0)
				buf[11] = 0x00 // (1000 >> 24) & 0xFF (0)
				return buf
			}(),
			err: "buffer too small for audio data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var af AudioFrame
			err := af.Unmarshal(tt.data)
			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.err)
			} else if err.Error()[:len(tt.err)] != tt.err {
				t.Errorf("Expected error containing '%s', got '%s'", tt.err, err.Error())
			}
		})
	}
}

func TestMarshalBinaryInterface(t *testing.T) {
	// Test that AudioFrame implements encoding.BinaryMarshaler and BinaryUnmarshaler
	original := AudioFrame{
		Format: FrameFormat{
			SampleRate:    44100,
			Channels:      2,
			BitsPerSample: 16,
		},
		Audio:        []byte{0xAA, 0xBB, 0xCC, 0xDD},
		SamplesCount: 2,
	}

	// Marshal using BinaryMarshaler interface
	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	// Unmarshal using BinaryUnmarshaler interface
	var decoded AudioFrame
	err = decoded.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}

	if !bytes.Equal(decoded.Audio, original.Audio) {
		t.Error("Audio data mismatch after BinaryMarshaler/Unmarshaler round-trip")
	}
}

func BenchmarkMarshal(b *testing.B) {
	af := AudioFrame{
		Format: FrameFormat{
			SampleRate:    44100,
			Channels:      2,
			BitsPerSample: 16,
		},
		Audio:        make([]byte, 4096),
		SamplesCount: 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = af.Marshal()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	af := AudioFrame{
		Format: FrameFormat{
			SampleRate:    44100,
			Channels:      2,
			BitsPerSample: 16,
		},
		Audio:        make([]byte, 4096),
		SamplesCount: 1024,
	}
	data := af.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded AudioFrame
		_ = decoded.Unmarshal(data)
	}
}
