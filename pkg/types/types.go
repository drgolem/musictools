package types

import (
	"time"

	"github.com/drgolem/ringbuffer"
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

// PlaybackMetrics provides detailed performance metrics for audio playback
// This structure captures comprehensive timing and performance data for
// monitoring, debugging, and optimizing audio playback.
type PlaybackMetrics struct {
	// Timing metrics
	ElapsedTime     time.Duration // Total wall-clock time since playback started
	PlayedSamples   uint64        // Samples actually sent to audio output
	BufferedSamples uint64        // Samples decoded but not yet played

	// Consumer (output) metrics
	ConsumerOps     uint64        // Total consumer loop iterations
	MaxConsumerTime time.Duration // Maximum time for a single consumer iteration
	AvgConsumerTime time.Duration // Average consumer iteration time
	OutputUnderruns uint64        // Count of buffer underruns (audio glitches)

	// Producer (decode) metrics
	ProducerOps     uint64        // Total producer loop iterations
	MaxProducerTime time.Duration // Maximum time for a single producer iteration
	AvgProducerTime time.Duration // Average producer iteration time
	DecodeErrors    uint64        // Count of decode errors encountered

	// Buffer metrics
	BufferSize        uint64  // Total ringbuffer capacity in bytes
	BufferAvailable   uint64  // Currently available bytes for reading
	BufferUtilization float64 // Buffer usage percentage (0-100)
	MaxBufferUsage    uint64  // Peak buffer usage in bytes

	// Audio format
	SampleRate    int // Current sample rate in Hz
	Channels      int // Current channel count
	BitsPerSample int // Current bit depth

	// Timing stability metrics
	MaxJitter time.Duration // Maximum timing jitter observed
	AvgJitter time.Duration // Average timing jitter
}

// ExtendedPlaybackStatus combines basic status with detailed metrics
type ExtendedPlaybackStatus struct {
	PlaybackStatus
	Metrics PlaybackMetrics
}

// Re-export common ringbuffer errors from github.com/drgolem/ringbuffer
// for backwards compatibility with existing code.
var (
	// ErrInsufficientSpace indicates the ringbuffer doesn't have enough space for the write operation
	ErrInsufficientSpace = ringbuffer.ErrInsufficientSpace

	// ErrInsufficientData indicates the ringbuffer doesn't have enough data for the read operation
	ErrInsufficientData = ringbuffer.ErrInsufficientData
)
