package audioplayer

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"learnRingbuffer/pkg/decoders/flac"
	"learnRingbuffer/pkg/decoders/mp3"
	"learnRingbuffer/pkg/decoders/wav"
	"learnRingbuffer/pkg/ringbuffer"
	"learnRingbuffer/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
)

// Player manages audio playback using producer/consumer pattern with ringbuffer
type Player struct {
	decoder         types.AudioDecoder
	ringbuf         *ringbuffer.RingBuffer
	stream          *portaudio.PaStream
	sampleRate      int
	channels        int
	bitsPerSample   int
	bytesPerSample  int
	framesPerBuffer int
	deviceIndex     int
	fileName        string
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	stopped         bool
	samplesConsumed atomic.Uint64
	startTime       time.Time
}

// Config holds player configuration
type Config struct {
	BufferSize      uint64 // Ringbuffer size in bytes
	FramesPerBuffer int    // Portaudio buffer size in frames
	DeviceIndex     int    // Audio output device index
}

// DefaultConfig returns default player configuration
func DefaultConfig() Config {
	return Config{
		BufferSize:      256 * 1024, // 256KB ringbuffer
		FramesPerBuffer: 512,        // 512 frames per buffer
		DeviceIndex:     1,          // Default device index
	}
}

// NewPlayer creates a new audio player
func NewPlayer(config Config) *Player {
	return &Player{
		ringbuf:         ringbuffer.New(config.BufferSize),
		framesPerBuffer: config.FramesPerBuffer,
		deviceIndex:     config.DeviceIndex,
		stopChan:        make(chan struct{}),
	}
}

// OpenFile opens an audio file for playback (auto-detects format)
func (p *Player) OpenFile(fileName string) error {
	// Try to detect file type by extension
	var decoder types.AudioDecoder
	ext := fileName[len(fileName)-4:]

	switch ext {
	case ".mp3":
		decoder = mp3.NewDecoder()
	case "flac", ".fla": // .flac or .fla
		decoder = flac.NewDecoder()
	case ".wav":
		decoder = wav.NewDecoder()
	default:
		return fmt.Errorf("unsupported file format: %s", ext)
	}

	// Open the file
	if err := decoder.Open(fileName); err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	// Get audio format
	rate, channels, bps := decoder.GetFormat()
	bytesPerSample := bps / 8

	slog.Info("Audio file opened",
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	p.decoder = decoder
	p.sampleRate = rate
	p.channels = channels
	p.bitsPerSample = bps
	p.bytesPerSample = bytesPerSample
	p.fileName = filepath.Base(fileName)

	return nil
}

// Play starts audio playback
func (p *Player) Play() error {
	if p.decoder == nil {
		return fmt.Errorf("no file opened")
	}

	// Initialize PortAudio stream
	if err := p.initStream(); err != nil {
		return fmt.Errorf("failed to initialize audio stream: %w", err)
	}

	// Start the audio stream
	if err := p.stream.StartStream(); err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	// Initialize playback tracking
	p.startTime = time.Now()
	p.samplesConsumed.Store(0)

	// Start producer goroutine (reads from file, writes to ringbuffer)
	p.wg.Add(1)
	go p.producer()

	// Start consumer goroutine (reads from ringbuffer, writes to portaudio)
	p.wg.Add(1)
	go p.consumer()

	slog.Info("Playback started")
	return nil
}

// Wait blocks until playback is complete
func (p *Player) Wait() {
	p.wg.Wait()
}

// Stop stops playback
func (p *Player) Stop() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	p.mu.Unlock()

	close(p.stopChan)
	p.wg.Wait()

	if p.stream != nil {
		if err := p.stream.StopStream(); err != nil {
			slog.Warn("Failed to stop stream", "error", err)
		}
		if err := p.stream.Close(); err != nil {
			slog.Warn("Failed to close stream", "error", err)
		}
	}

	if p.decoder != nil {
		if err := p.decoder.Close(); err != nil {
			slog.Warn("Failed to close decoder", "error", err)
		}
	}

	slog.Info("Playback stopped")
	return nil
}

// initStream initializes the PortAudio stream
func (p *Player) initStream() error {
	// Determine sample format based on bit depth
	var sampleFormat portaudio.PaSampleFormat
	switch p.bitsPerSample {
	case 16:
		sampleFormat = portaudio.SampleFmtInt16
	case 24:
		sampleFormat = portaudio.SampleFmtInt24
	case 32:
		sampleFormat = portaudio.SampleFmtInt32
	default:
		return fmt.Errorf("unsupported bit depth: %d", p.bitsPerSample)
	}

	// Configure output stream parameters
	outParams := portaudio.PaStreamParameters{
		DeviceIndex:  p.deviceIndex,
		ChannelCount: p.channels,
		SampleFormat: sampleFormat,
	}

	// Create stream
	stream, err := portaudio.NewStream(outParams, float64(p.sampleRate))
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Open the stream
	if err := stream.Open(p.framesPerBuffer); err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	p.stream = stream
	return nil
}

// consumer reads from ringbuffer and writes to portaudio
// This goroutine pulls data from the ringbuffer and writes to audio output
func (p *Player) consumer() {
	defer p.wg.Done()

	framesPerBuffer := p.framesPerBuffer
	bytesPerFrame := p.channels * p.bytesPerSample
	bufferSize := framesPerBuffer * bytesPerFrame
	buffer := make([]byte, bufferSize)

	slog.Info("Consumer started")

	for {
		select {
		case <-p.stopChan:
			slog.Info("Consumer stopped")
			return
		default:
		}

		// Read from ringbuffer directly into buffer
		// Read() handles wrap-around internally - same performance as manual ReadSlices()
		bytesRead, err := p.ringbuf.Read(buffer)

		if err != nil {
			// Buffer underrun - wait a bit
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Align to frame boundary
		frames := bytesRead / bytesPerFrame
		if frames == 0 {
			// Not enough for even one frame, wait
			time.Sleep(1 * time.Millisecond)
			continue
		}
		bytesAligned := frames * bytesPerFrame

		// Write to portaudio
		err = p.stream.Write(frames, buffer[:bytesAligned])
		if err != nil {
			slog.Error("Failed to write to audio stream", "error", err)
			return
		}

		// Track samples consumed (frames written to output)
		samplesWritten := uint64(frames)
		p.samplesConsumed.Add(samplesWritten)
	}
}

// producer reads from decoder and writes to ringbuffer
func (p *Player) producer() {
	defer p.wg.Done()

	audioSamples := 4 * 1024 // Decode 4K samples at a time
	bufferBytes := audioSamples * p.channels * p.bytesPerSample
	buffer := make([]byte, bufferBytes)

	slog.Info("Producer started")

	for {
		select {
		case <-p.stopChan:
			slog.Info("Producer stopped")
			return
		default:
		}

		// Decode samples from file
		samplesRead, err := p.decoder.DecodeSamples(audioSamples, buffer)
		if err != nil || samplesRead == 0 {
			// End of file or error
			slog.Info("Producer finished", "error", err, "samples", samplesRead)
			time.Sleep(2 * time.Second) // Let buffer drain
			p.Stop()
			return
		}

		// Calculate bytes to write
		bytesToWrite := samplesRead * p.channels * p.bytesPerSample

		// Write to ringbuffer (blocks if buffer is full)
		for {
			_, err := p.ringbuf.Write(buffer[:bytesToWrite])
			if err == nil {
				break // Successfully written
			}

			// Buffer full, wait a bit
			select {
			case <-p.stopChan:
				return
			case <-time.After(10 * time.Millisecond):
				// Try again
			}
		}
	}
}

// GetBufferStatus returns current ringbuffer status
func (p *Player) GetBufferStatus() (available, size uint64) {
	return p.ringbuf.AvailableRead(), p.ringbuf.Size()
}

// GetPlaybackStatus returns current playback status
func (p *Player) GetPlaybackStatus() types.PlaybackStatus {
	samples := p.samplesConsumed.Load()
	elapsed := time.Since(p.startTime)

	return types.PlaybackStatus{
		FileName:        filepath.Base(p.fileName),
		SampleRate:      p.sampleRate,
		Channels:        p.channels,
		BitsPerSample:   p.bitsPerSample,
		FramesPerBuffer: p.framesPerBuffer,
		PlayedSamples:   samples, // This player tracks consumed samples as played
		BufferedSamples: 0,       // This simple player doesn't track buffering separately
		ElapsedTime:     elapsed,
	}
}
