package audioframe

import (
	"encoding/binary"
	"fmt"
)

type FrameFormat struct {
	SampleRate    uint32 // Sample rate in Hz (max 384,000)
	Channels      uint8  // Number of channels (max 10)
	BitsPerSample uint8  // Bits per sample (max 64)
}

type AudioFrame struct {
	Format       FrameFormat
	SamplesCount uint16 // Number of samples (max 65,535)
	Audio        []byte // Raw audio data (last field for better memory layout)
}

// Marshal serializes AudioFrame to a byte slice using little-endian encoding
//
// Binary format (tightly packed, 12 bytes header):
//   - SampleRate (4 bytes, uint32)
//   - Channels (1 byte, uint8)
//   - BitsPerSample (1 byte, uint8)
//   - SamplesCount (2 bytes, uint16)
//   - Audio length (4 bytes, uint32)
//   - Audio data (variable length)
//
// Total size: 12 bytes header + len(Audio) bytes
func (af *AudioFrame) Marshal() []byte {
	// Calculate total size: 4 + 1 + 1 + 2 + 4 = 12 bytes header + audio data
	headerSize := 12
	totalSize := headerSize + len(af.Audio)
	buf := make([]byte, totalSize)

	// Write header fields using little-endian (tightly packed)
	binary.LittleEndian.PutUint32(buf[0:4], af.Format.SampleRate)
	buf[4] = af.Format.Channels
	buf[5] = af.Format.BitsPerSample
	binary.LittleEndian.PutUint16(buf[6:8], af.SamplesCount)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(len(af.Audio)))

	// Copy audio data
	copy(buf[12:], af.Audio)

	return buf
}

// Unmarshal deserializes a byte slice into AudioFrame using little-endian encoding
//
// Returns error if:
//   - Buffer is too small (< 12 bytes for header)
//   - Audio length field exceeds remaining buffer size
func (af *AudioFrame) Unmarshal(data []byte) error {
	// Check minimum size for header
	headerSize := 12
	if len(data) < headerSize {
		return fmt.Errorf("buffer too small: got %d bytes, need at least %d bytes", len(data), headerSize)
	}

	// Read header fields (tightly packed)
	af.Format.SampleRate = binary.LittleEndian.Uint32(data[0:4])
	af.Format.Channels = data[4]
	af.Format.BitsPerSample = data[5]
	af.SamplesCount = binary.LittleEndian.Uint16(data[6:8])
	audioLen := int(binary.LittleEndian.Uint32(data[8:12]))

	// Validate audio length
	if len(data) < headerSize+audioLen {
		return fmt.Errorf("buffer too small for audio data: got %d bytes, need %d bytes", len(data), headerSize+audioLen)
	}

	// Allocate and copy audio data
	af.Audio = make([]byte, audioLen)
	copy(af.Audio, data[12:12+audioLen])

	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler interface
func (af *AudioFrame) MarshalBinary() ([]byte, error) {
	return af.Marshal(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler interface
func (af *AudioFrame) UnmarshalBinary(data []byte) error {
	return af.Unmarshal(data)
}
