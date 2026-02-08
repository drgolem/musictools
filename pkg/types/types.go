package types

import (
	"errors"
	"time"
)

// AudioDecoder is the common interface for all audio decoders (MP3, FLAC, WAV).
// All decoders must implement these methods to provide a consistent API
// for decoding audio files into raw PCM samples.
type AudioDecoder interface {
	// Open opens an audio file for decoding
	Open(fileName string) error

	// Close closes the decoder and releases resources
	Close() error

	// GetFormat returns the audio format information
	// Returns: sample rate (Hz), channels (1=mono, 2=stereo), bits per sample (8/16/24/32)
	GetFormat() (rate, channels, bitsPerSample int)

	// DecodeSamples decodes audio samples into the provided buffer
	// Parameters:
	//   samples: number of samples to decode (not bytes!)
	//   audio: buffer to write decoded audio data
	// Returns: number of samples actually decoded, error if decoding failed
	// Note: Buffer must be large enough: samples * channels * (bitsPerSample/8) bytes
	DecodeSamples(samples int, audio []byte) (int, error)
}

// PlaybackStatus holds unified playback information for audio players.
// This struct provides real-time metrics for monitoring audio playback.
type PlaybackStatus struct {
	FileName        string        // Name of the currently playing file
	SampleRate      int           // Audio sample rate in Hz (e.g., 44100, 48000)
	Channels        int           // Number of audio channels (1=mono, 2=stereo)
	BitsPerSample   int           // Bit depth (8, 16, 24, or 32)
	FramesPerBuffer int           // PortAudio frames per buffer (if applicable)
	PlayedSamples   uint64        // Samples actually sent to audio output (played)
	BufferedSamples uint64        // Samples decoded but not yet played (in-flight)
	ElapsedTime     time.Duration // Wall-clock time since playback started
}

// PlaybackMonitor is an interface for types that can report playback status.
// Implementing this interface allows consistent status monitoring across
// different player implementations.
type PlaybackMonitor interface {
	GetPlaybackStatus() PlaybackStatus
}

// Common ringbuffer errors used by both byte-based and frame-based ringbuffers.
// These errors enable consistent error handling and comparison using errors.Is().
var (
	// ErrInsufficientSpace indicates the ringbuffer doesn't have enough space for the write operation
	ErrInsufficientSpace = errors.New("insufficient space in ringbuffer")

	// ErrInsufficientData indicates the ringbuffer doesn't have enough data for the read operation
	ErrInsufficientData = errors.New("insufficient data in ringbuffer")
)
