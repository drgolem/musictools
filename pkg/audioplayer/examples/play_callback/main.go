package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/drgolem/musictools/pkg/decoders/flac"
	"github.com/drgolem/musictools/pkg/decoders/mp3"
	"github.com/drgolem/musictools/pkg/decoders/wav"
	"github.com/drgolem/ringbuffer"
	"github.com/drgolem/musictools/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
)

// CallbackPlayer demonstrates callback-based audio playback using ringbuffer
type CallbackPlayer struct {
	decoder         types.AudioDecoder
	ringbuf         *ringbuffer.RingBuffer
	stream          *portaudio.PaStream
	sampleRate      int
	channels        int
	bitsPerSample   int
	bytesPerSample  int
	framesPerBuffer int
	deviceIndex     int
	producerDone    atomic.Bool
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	stopped         bool
}

func NewCallbackPlayer(deviceIdx int, bufferSize uint64, framesPerBuffer int) *CallbackPlayer {
	return &CallbackPlayer{
		ringbuf:         ringbuffer.New(bufferSize),
		framesPerBuffer: framesPerBuffer,
		deviceIndex:     deviceIdx,
		stopChan:        make(chan struct{}),
	}
}

func (cp *CallbackPlayer) OpenFile(fileName string) error {
	// Auto-detect file type
	var decoder types.AudioDecoder
	ext := fileName[len(fileName)-4:]

	switch ext {
	case ".mp3":
		decoder = mp3.NewDecoder()
	case "flac", ".fla":
		decoder = flac.NewDecoder()
	case ".wav":
		decoder = wav.NewDecoder()
	default:
		return fmt.Errorf("unsupported file format: %s", ext)
	}

	if err := decoder.Open(fileName); err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	rate, channels, bps := decoder.GetFormat()
	bytesPerSample := bps / 8

	slog.Info("Audio file opened",
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	cp.decoder = decoder
	cp.sampleRate = rate
	cp.channels = channels
	cp.bitsPerSample = bps
	cp.bytesPerSample = bytesPerSample

	return nil
}

func (cp *CallbackPlayer) Play() error {
	if cp.decoder == nil {
		return fmt.Errorf("no file opened")
	}

	// Determine sample format
	var sampleFormat portaudio.PaSampleFormat
	switch cp.bitsPerSample {
	case 16:
		sampleFormat = portaudio.SampleFmtInt16
	case 24:
		sampleFormat = portaudio.SampleFmtInt24
	case 32:
		sampleFormat = portaudio.SampleFmtInt32
	default:
		return fmt.Errorf("unsupported bit depth: %d", cp.bitsPerSample)
	}

	// Create stream
	cp.stream = &portaudio.PaStream{
		OutputParameters: &portaudio.PaStreamParameters{
			DeviceIndex:  cp.deviceIndex,
			ChannelCount: cp.channels,
			SampleFormat: sampleFormat,
		},
		SampleRate: float64(cp.sampleRate),
		// UseHighLatency: false is default for callbacks (low latency)
	}

	// Open stream with callback
	if err := cp.stream.OpenCallback(cp.framesPerBuffer, cp.audioCallback); err != nil {
		return fmt.Errorf("failed to open stream with callback: %w", err)
	}

	// Start the stream
	if err := cp.stream.StartStream(); err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	// Start producer goroutine (fills ringbuffer from file)
	cp.wg.Add(1)
	go cp.producer()

	slog.Info("Playback started (callback mode)")
	return nil
}

// audioCallback is called by PortAudio to fill the output buffer
// This runs in real-time context - must be fast and avoid allocations
func (cp *CallbackPlayer) audioCallback(
	input, output []byte,
	frameCount uint,
	timeInfo *portaudio.StreamCallbackTimeInfo,
	statusFlags portaudio.StreamCallbackFlags,
) portaudio.StreamCallbackResult {

	bytesNeeded := int(frameCount) * cp.channels * cp.bytesPerSample

	// Check if producer is done and buffer is empty
	if cp.producerDone.Load() && cp.ringbuf.AvailableRead() == 0 {
		return portaudio.Complete
	}

	// Read from ringbuffer directly into output buffer
	// Read() handles wrap-around internally - same performance as manual ReadSlices()
	n, _ := cp.ringbuf.Read(output[:bytesNeeded])

	// Fill remainder with silence if we got less than needed
	if n < bytesNeeded {
		clear(output[n:bytesNeeded])
	}

	return portaudio.Continue
}

// producer reads from decoder and writes to ringbuffer
func (cp *CallbackPlayer) producer() {
	defer cp.wg.Done()
	defer cp.producerDone.Store(true)

	audioSamples := 4 * 1024
	bufferBytes := audioSamples * cp.channels * cp.bytesPerSample
	buffer := make([]byte, bufferBytes)

	slog.Info("Producer started")

	for {
		select {
		case <-cp.stopChan:
			slog.Info("Producer stopped")
			return
		default:
		}

		// Decode samples
		samplesRead, err := cp.decoder.DecodeSamples(audioSamples, buffer)
		if err != nil || samplesRead == 0 {
			slog.Info("Producer finished", "error", err, "samples", samplesRead)
			return
		}

		bytesToWrite := samplesRead * cp.channels * cp.bytesPerSample

		// Write to ringbuffer (wait if full)
		for {
			_, err := cp.ringbuf.Write(buffer[:bytesToWrite])
			if err == nil {
				break
			}

			// Check if stopped
			select {
			case <-cp.stopChan:
				return
			default:
			}

			// Buffer full - callback will drain it
			// Small sleep to avoid busy waiting
		}
	}
}

func (cp *CallbackPlayer) Wait() {
	cp.wg.Wait()
}

func (cp *CallbackPlayer) Stop() error {
	cp.mu.Lock()
	if cp.stopped {
		cp.mu.Unlock()
		return nil
	}
	cp.stopped = true
	cp.mu.Unlock()

	close(cp.stopChan)
	cp.wg.Wait()

	if cp.stream != nil {
		if err := cp.stream.StopStream(); err != nil {
			slog.Warn("Failed to stop stream", "error", err)
		}
		if err := cp.stream.CloseCallback(); err != nil {
			slog.Warn("Failed to close stream", "error", err)
		}
	}

	if cp.decoder != nil {
		if err := cp.decoder.Close(); err != nil {
			slog.Warn("Failed to close decoder", "error", err)
		}
	}

	slog.Info("Playback stopped")
	return nil
}

func (cp *CallbackPlayer) GetBufferStatus() (available, size uint64) {
	return cp.ringbuf.AvailableRead(), cp.ringbuf.Size()
}

func main() {
	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Parse flags
	deviceIdx := flag.Int("device", 1, "Audio output device index")
	bufferSize := flag.Uint64("buffer", 256*1024, "Ringbuffer size in bytes")
	frames := flag.Int("frames", 512, "Audio frames per buffer")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: play_callback [options] <audio_file>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Plays audio using PortAudio callback mode with ringbuffer")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  play_callback music.mp3")
		fmt.Fprintln(os.Stderr, "  play_callback -device 0 -v music.flac")
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	fileName := flag.Arg(0)

	if *verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
	}

	// Initialize PortAudio
	slog.Info("Initializing PortAudio")
	if err := portaudio.Initialize(); err != nil {
		slog.Error("Failed to initialize PortAudio", "error", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	slog.Info("PortAudio initialized",
		"version", portaudio.GetVersion())
	slog.Info("Configuration",
		"device_index", *deviceIdx,
		"buffer_size", *bufferSize,
		"frames_per_buffer", *frames)

	// Create callback player
	player := NewCallbackPlayer(*deviceIdx, *bufferSize, *frames)

	// Open file
	slog.Info("Opening file", "path", fileName)
	if err := player.OpenFile(fileName); err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	// Setup signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start playback
	if err := player.Play(); err != nil {
		slog.Error("Failed to start playback", "error", err)
		os.Exit(1)
	}

	// Wait for completion or interrupt
	done := make(chan struct{})
	go func() {
		player.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Playback completed")
	case sig := <-sigChan:
		slog.Info("Signal received, stopping", "signal", sig)
		if err := player.Stop(); err != nil {
			slog.Error("Failed to stop player", "error", err)
		}
	}

	slog.Info("Exiting")
}
