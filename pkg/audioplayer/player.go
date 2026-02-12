package audioplayer

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drgolem/musictools/pkg/decoders/flac"
	"github.com/drgolem/musictools/pkg/decoders/mp3"
	"github.com/drgolem/musictools/pkg/decoders/wav"
	"github.com/drgolem/musictools/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
	"github.com/drgolem/ringbuffer"
)

// AudioFormatSnapshot captures current audio format state for change detection
type AudioFormatSnapshot struct {
	SampleRate     int
	Channels       int
	BitsPerSample  int
	BytesPerSample int
}

// Player manages audio playback using producer/consumer pattern with ringbuffer
// Enhanced with dynamic format switching and comprehensive metrics
type Player struct {
	decoder         types.AudioDecoder
	ringbuf         *ringbuffer.RingBuffer
	stream          *portaudio.PaStream
	streamMx        sync.Mutex // Protects stream during reconfiguration
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

	// Format change handling
	currentFormat AudioFormatSnapshot
	formatMx      sync.RWMutex

	// Metrics tracking
	metrics struct {
		sync.RWMutex

		// Consumer metrics
		consumerOps      atomic.Uint64
		consumerTimeSum  atomic.Uint64 // Microseconds
		maxConsumerTime  time.Duration
		lastConsumerTime time.Time
		outputUnderruns  atomic.Uint64

		// Producer metrics
		producerOps      atomic.Uint64
		producerTimeSum  atomic.Uint64 // Microseconds
		maxProducerTime  time.Duration
		decodeErrors     atomic.Uint64

		// Buffer metrics
		maxBufferUsage atomic.Uint64

		// Jitter tracking
		maxJitter  time.Duration
		jitterSum  atomic.Uint64 // Microseconds
		jitterOps  atomic.Uint64
	}
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

	p.fileName = filepath.Base(fileName)
	return p.OpenDecoder(decoder)
}

// OpenDecoder opens an audio decoder for playback
// This allows custom decoder implementations for streaming, network sources, etc.
func (p *Player) OpenDecoder(decoder types.AudioDecoder) error {
	// Get audio format
	rate, channels, bps := decoder.GetFormat()
	bytesPerSample := bps / 8

	slog.Info("Audio decoder opened",
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	p.decoder = decoder
	p.sampleRate = rate
	p.channels = channels
	p.bitsPerSample = bps
	p.bytesPerSample = bytesPerSample

	// Initialize format snapshot
	p.updateFormat(AudioFormatSnapshot{
		SampleRate:     rate,
		Channels:       channels,
		BitsPerSample:  bps,
		BytesPerSample: bytesPerSample,
	})

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

// getCurrentFormat safely retrieves the current format snapshot
func (p *Player) getCurrentFormat() AudioFormatSnapshot {
	p.formatMx.RLock()
	defer p.formatMx.RUnlock()
	return p.currentFormat
}

// updateFormat safely updates the current format snapshot
func (p *Player) updateFormat(snapshot AudioFormatSnapshot) {
	p.formatMx.Lock()
	defer p.formatMx.Unlock()
	p.currentFormat = snapshot
}

// reconfigureStreamIfNeeded checks if format changed and reconfigures PortAudio
func (p *Player) reconfigureStreamIfNeeded(newRate, newChannels, newBPS int) error {
	currentFormat := p.getCurrentFormat()

	// Check if format changed
	if currentFormat.SampleRate == newRate &&
		currentFormat.Channels == newChannels &&
		currentFormat.BitsPerSample == newBPS {
		return nil // No change
	}

	slog.Info("Audio format changed, reconfiguring stream",
		"old_rate", currentFormat.SampleRate,
		"new_rate", newRate,
		"old_channels", currentFormat.Channels,
		"new_channels", newChannels,
		"old_bps", currentFormat.BitsPerSample,
		"new_bps", newBPS)

	p.streamMx.Lock()
	defer p.streamMx.Unlock()

	// Stop and close old stream
	if p.stream != nil {
		if err := p.stream.StopStream(); err != nil {
			slog.Warn("Failed to stop old stream", "error", err)
		}
		if err := p.stream.Close(); err != nil {
			slog.Warn("Failed to close old stream", "error", err)
		}
	}

	// Update format
	p.sampleRate = newRate
	p.channels = newChannels
	p.bitsPerSample = newBPS
	p.bytesPerSample = newBPS / 8

	p.updateFormat(AudioFormatSnapshot{
		SampleRate:     newRate,
		Channels:       newChannels,
		BitsPerSample:  newBPS,
		BytesPerSample: newBPS / 8,
	})

	// Create and start new stream
	if err := p.initStream(); err != nil {
		return fmt.Errorf("failed to reinitialize stream: %w", err)
	}

	if err := p.stream.StartStream(); err != nil {
		return fmt.Errorf("failed to start reconfigured stream: %w", err)
	}

	slog.Info("Stream reconfigured successfully")
	return nil
}

// consumer reads from ringbuffer and writes to portaudio
// This goroutine pulls data from the ringbuffer and writes to audio output
func (p *Player) consumer() {
	defer p.wg.Done()

	framesPerBuffer := p.framesPerBuffer
	buffer := make([]byte, framesPerBuffer*8*2) // Max size for 2ch 32-bit

	slog.Info("Consumer started")

	var lastWriteTime time.Time
	expectedInterval := time.Duration(0)

	for {
		iterStart := time.Now()

		select {
		case <-p.stopChan:
			slog.Info("Consumer stopped")
			return
		default:
		}

		// Check for format changes from decoder
		rate, channels, bps := p.decoder.GetFormat()
		if err := p.reconfigureStreamIfNeeded(rate, channels, bps); err != nil {
			slog.Error("Failed to reconfigure stream", "error", err)
			return
		}

		currentFormat := p.getCurrentFormat()
		bytesPerFrame := currentFormat.Channels * currentFormat.BytesPerSample
		bufferSize := framesPerBuffer * bytesPerFrame

		// Calculate expected interval for jitter tracking
		if expectedInterval == 0 {
			expectedInterval = time.Duration(float64(framesPerBuffer)/float64(currentFormat.SampleRate)*float64(time.Second))
		}

		// Ensure buffer is large enough
		if len(buffer) < bufferSize {
			buffer = make([]byte, bufferSize)
		}

		// Read from ringbuffer
		readStart := time.Now()
		bytesRead, err := p.ringbuf.Read(buffer[:bufferSize])
		readTime := time.Since(readStart)

		if err != nil {
			// Buffer underrun - wait a bit
			p.metrics.outputUnderruns.Add(1)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Track buffer usage
		available := p.ringbuf.AvailableRead()
		p.updateMaxBufferUsage(available)

		// Align to frame boundary
		frames := bytesRead / bytesPerFrame
		if frames == 0 {
			// Not enough for even one frame, wait
			time.Sleep(1 * time.Millisecond)
			continue
		}
		bytesAligned := frames * bytesPerFrame

		// Write to portaudio (with stream lock for reconfiguration safety)
		writeStart := time.Now()
		p.streamMx.Lock()
		err = p.stream.Write(frames, buffer[:bytesAligned])
		p.streamMx.Unlock()
		writeTime := time.Since(writeStart)

		if err != nil {
			slog.Error("Failed to write to audio stream", "error", err)
			return
		}

		// Update metrics
		iterTime := time.Since(iterStart)
		p.updateConsumerMetrics(iterTime, readTime, writeTime)

		// Track jitter (timing stability)
		if !lastWriteTime.IsZero() {
			now := time.Now()
			actualInterval := now.Sub(lastWriteTime)
			jitter := actualInterval - expectedInterval
			if jitter < 0 {
				jitter = -jitter
			}
			p.updateJitterMetrics(jitter)
		}
		lastWriteTime = time.Now()

		// Track samples consumed (frames written to output)
		samplesWritten := uint64(frames)
		p.samplesConsumed.Add(samplesWritten)
	}
}

// producer reads from decoder and writes to ringbuffer
func (p *Player) producer() {
	defer p.wg.Done()

	audioSamples := 4 * 1024 // Decode 4K samples at a time
	bufferBytes := audioSamples * 8 * 2 // Max for 2ch 32-bit
	buffer := make([]byte, bufferBytes)

	slog.Info("Producer started")

	for {
		iterStart := time.Now()

		select {
		case <-p.stopChan:
			slog.Info("Producer stopped")
			return
		default:
		}

		// Get current format for buffer sizing
		currentFormat := p.getCurrentFormat()
		bufferBytes = audioSamples * currentFormat.Channels * currentFormat.BytesPerSample
		if len(buffer) < bufferBytes {
			buffer = make([]byte, bufferBytes)
		}

		// Decode samples from file
		decodeStart := time.Now()
		samplesRead, err := p.decoder.DecodeSamples(audioSamples, buffer)
		decodeTime := time.Since(decodeStart)

		if err != nil || samplesRead == 0 {
			// End of file or error
			if err != nil {
				p.metrics.decodeErrors.Add(1)
			}
			slog.Info("Producer finished", "error", err, "samples", samplesRead)
			time.Sleep(2 * time.Second) // Let buffer drain
			p.Stop()
			return
		}

		// Calculate bytes to write
		bytesToWrite := samplesRead * currentFormat.Channels * currentFormat.BytesPerSample

		// Write to ringbuffer (blocks if buffer is full)
		writeStart := time.Now()
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
		writeTime := time.Since(writeStart)

		// Update metrics
		iterTime := time.Since(iterStart)
		p.updateProducerMetrics(iterTime, decodeTime, writeTime)
	}
}

// Metrics helper methods
func (p *Player) updateConsumerMetrics(totalTime, readTime, writeTime time.Duration) {
	p.metrics.consumerOps.Add(1)
	p.metrics.consumerTimeSum.Add(uint64(totalTime.Microseconds()))

	p.metrics.Lock()
	if totalTime > p.metrics.maxConsumerTime {
		p.metrics.maxConsumerTime = totalTime
	}
	p.metrics.Unlock()
}

func (p *Player) updateProducerMetrics(totalTime, decodeTime, writeTime time.Duration) {
	p.metrics.producerOps.Add(1)
	p.metrics.producerTimeSum.Add(uint64(totalTime.Microseconds()))

	p.metrics.Lock()
	if totalTime > p.metrics.maxProducerTime {
		p.metrics.maxProducerTime = totalTime
	}
	p.metrics.Unlock()
}

func (p *Player) updateMaxBufferUsage(current uint64) {
	for {
		old := p.metrics.maxBufferUsage.Load()
		if current <= old {
			break
		}
		if p.metrics.maxBufferUsage.CompareAndSwap(old, current) {
			break
		}
	}
}

func (p *Player) updateJitterMetrics(jitter time.Duration) {
	p.metrics.jitterOps.Add(1)
	p.metrics.jitterSum.Add(uint64(jitter.Microseconds()))

	p.metrics.Lock()
	if jitter > p.metrics.maxJitter {
		p.metrics.maxJitter = jitter
	}
	p.metrics.Unlock()
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
		PlayedSamples:   samples,
		BufferedSamples: 0,
		ElapsedTime:     elapsed,
	}
}

// GetExtendedPlaybackStatus returns comprehensive playback metrics
func (p *Player) GetExtendedPlaybackStatus() types.ExtendedPlaybackStatus {
	basicStatus := p.GetPlaybackStatus()

	p.metrics.RLock()
	defer p.metrics.RUnlock()

	// Calculate averages
	consumerOps := p.metrics.consumerOps.Load()
	avgConsumerTime := time.Duration(0)
	if consumerOps > 0 {
		avgConsumerTime = time.Duration(p.metrics.consumerTimeSum.Load()/consumerOps) * time.Microsecond
	}

	producerOps := p.metrics.producerOps.Load()
	avgProducerTime := time.Duration(0)
	if producerOps > 0 {
		avgProducerTime = time.Duration(p.metrics.producerTimeSum.Load()/producerOps) * time.Microsecond
	}

	jitterOps := p.metrics.jitterOps.Load()
	avgJitter := time.Duration(0)
	if jitterOps > 0 {
		avgJitter = time.Duration(p.metrics.jitterSum.Load()/jitterOps) * time.Microsecond
	}

	bufferSize := p.ringbuf.Size()
	bufferAvailable := p.ringbuf.AvailableRead()
	bufferUtilization := float64(bufferAvailable) / float64(bufferSize) * 100.0

	return types.ExtendedPlaybackStatus{
		PlaybackStatus: basicStatus,
		Metrics: types.PlaybackMetrics{
			ElapsedTime:     basicStatus.ElapsedTime,
			PlayedSamples:   basicStatus.PlayedSamples,
			BufferedSamples: basicStatus.BufferedSamples,

			ConsumerOps:     consumerOps,
			MaxConsumerTime: p.metrics.maxConsumerTime,
			AvgConsumerTime: avgConsumerTime,
			OutputUnderruns: p.metrics.outputUnderruns.Load(),

			ProducerOps:     producerOps,
			MaxProducerTime: p.metrics.maxProducerTime,
			AvgProducerTime: avgProducerTime,
			DecodeErrors:    p.metrics.decodeErrors.Load(),

			BufferSize:        bufferSize,
			BufferAvailable:   bufferAvailable,
			BufferUtilization: bufferUtilization,
			MaxBufferUsage:    p.metrics.maxBufferUsage.Load(),

			SampleRate:    p.sampleRate,
			Channels:      p.channels,
			BitsPerSample: p.bitsPerSample,

			MaxJitter: p.metrics.maxJitter,
			AvgJitter: avgJitter,
		},
	}
}

// PrintMetrics outputs formatted metrics to console
func (p *Player) PrintMetrics() {
	status := p.GetExtendedPlaybackStatus()
	m := status.Metrics

	fmt.Println("\n=== Playback Metrics ===")
	fmt.Printf("Elapsed Time:     %v\n", m.ElapsedTime)
	fmt.Printf("Samples Played:   %d\n", m.PlayedSamples)

	fmt.Println("\n--- Consumer (Output) ---")
	fmt.Printf("Operations:       %d\n", m.ConsumerOps)
	fmt.Printf("Max Latency:      %v\n", m.MaxConsumerTime)
	fmt.Printf("Avg Latency:      %v\n", m.AvgConsumerTime)
	fmt.Printf("Underruns:        %d\n", m.OutputUnderruns)

	fmt.Println("\n--- Producer (Decode) ---")
	fmt.Printf("Operations:       %d\n", m.ProducerOps)
	fmt.Printf("Max Decode Time:  %v\n", m.MaxProducerTime)
	fmt.Printf("Avg Decode Time:  %v\n", m.AvgProducerTime)
	fmt.Printf("Decode Errors:    %d\n", m.DecodeErrors)

	fmt.Println("\n--- Buffer Stats ---")
	fmt.Printf("Buffer Size:      %d bytes\n", m.BufferSize)
	fmt.Printf("Available:        %d bytes\n", m.BufferAvailable)
	fmt.Printf("Utilization:      %.1f%%\n", m.BufferUtilization)
	fmt.Printf("Peak Usage:       %d bytes\n", m.MaxBufferUsage)

	fmt.Println("\n--- Timing Stability ---")
	fmt.Printf("Max Jitter:       %v\n", m.MaxJitter)
	fmt.Printf("Avg Jitter:       %v\n", m.AvgJitter)
}
