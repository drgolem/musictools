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
	"time"

	"musictools/pkg/audioframe"
	"musictools/pkg/audioframeringbuffer"
	"musictools/pkg/decoders/flac"
	"musictools/pkg/decoders/mp3"
	"musictools/pkg/decoders/wav"
	"musictools/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
)

// FramesPlayer demonstrates callback-based audio playback using AudioFrameRingBuffer
// This shows frame-based buffering where each frame includes format metadata
type FramesPlayer struct {
	decoder         types.AudioDecoder
	ringbuf         *audioframeringbuffer.AudioFrameRingBuffer
	stream          *portaudio.PaStream
	sampleRate      int
	channels        int
	bitsPerSample   int
	bytesPerSample  int
	framesPerBuffer int
	samplesPerFrame int // samples per AudioFrame
	deviceIndex     int
	producerDone    atomic.Bool
	playbackComplete atomic.Bool
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	stopped         bool

	// Callback state for partial frame consumption
	currentFrame *audioframe.AudioFrame
	frameOffset  int // bytes consumed from currentFrame

	// Format change handling
	formatChangePending atomic.Bool
	pendingFrame        *audioframe.AudioFrame
	frameMu             sync.Mutex
}

func NewFramesPlayer(deviceIdx int, bufferCapacity uint64, framesPerBuffer, samplesPerFrame int) *FramesPlayer {
	return &FramesPlayer{
		ringbuf:         audioframeringbuffer.New(bufferCapacity),
		framesPerBuffer: framesPerBuffer,
		samplesPerFrame: samplesPerFrame,
		deviceIndex:     deviceIdx,
		stopChan:        make(chan struct{}),
	}
}

func (fp *FramesPlayer) OpenFile(fileName string) error {
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
		"bits_per_sample", bps,
		"samples_per_frame", fp.samplesPerFrame)

	fp.decoder = decoder
	fp.sampleRate = rate
	fp.channels = channels
	fp.bitsPerSample = bps
	fp.bytesPerSample = bytesPerSample

	return nil
}

func (fp *FramesPlayer) Play() error {
	if fp.decoder == nil {
		return fmt.Errorf("no file opened")
	}

	// Reset playback completion flag
	fp.playbackComplete.Store(false)

	// Start producer goroutine (fills ringbuffer with AudioFrames)
	fp.wg.Add(1)
	go fp.producer()

	// Initialize and start stream
	if err := fp.initializeStream(); err != nil {
		return err
	}

	slog.Info("Playback started (frame-based callback mode)")

	return nil
}

func (fp *FramesPlayer) initializeStream() error {
	// Determine sample format
	var sampleFormat portaudio.PaSampleFormat
	switch fp.bitsPerSample {
	case 16:
		sampleFormat = portaudio.SampleFmtInt16
	case 24:
		sampleFormat = portaudio.SampleFmtInt24
	case 32:
		sampleFormat = portaudio.SampleFmtInt32
	default:
		return fmt.Errorf("unsupported bit depth: %d", fp.bitsPerSample)
	}

	// Create stream
	fp.stream = &portaudio.PaStream{
		OutputParameters: &portaudio.PaStreamParameters{
			DeviceIndex:  fp.deviceIndex,
			ChannelCount: fp.channels,
			SampleFormat: sampleFormat,
		},
		SampleRate: float64(fp.sampleRate),
	}

	// Open stream with callback
	if err := fp.stream.OpenCallback(fp.framesPerBuffer, fp.audioCallback); err != nil {
		return fmt.Errorf("failed to open stream with callback: %w", err)
	}

	// Start the stream
	if err := fp.stream.StartStream(); err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	slog.Info("PortAudio stream initialized",
		"sample_rate", fp.sampleRate,
		"channels", fp.channels,
		"bits_per_sample", fp.bitsPerSample)

	return nil
}

func (fp *FramesPlayer) reinitializeStream(newFrame *audioframe.AudioFrame) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	// Close old stream
	if fp.stream != nil {
		if err := fp.stream.StopStream(); err != nil {
			slog.Warn("Failed to stop old stream", "error", err)
		}
		if err := fp.stream.CloseCallback(); err != nil {
			slog.Warn("Failed to close old stream", "error", err)
		}
	}

	// Update format from new frame
	fp.sampleRate = int(newFrame.Format.SampleRate)
	fp.channels = int(newFrame.Format.Channels)
	fp.bitsPerSample = int(newFrame.Format.BitsPerSample)
	fp.bytesPerSample = fp.bitsPerSample / 8

	slog.Info("Reinitializing stream with new format",
		"sample_rate", fp.sampleRate,
		"channels", fp.channels,
		"bits_per_sample", fp.bitsPerSample)

	// Reinitialize with new format
	return fp.initializeStream()
}


// audioCallback is called by PortAudio to fill the output buffer.
//
// IMPORTANT: This runs in a separate audio thread managed by PortAudio's C library,
// NOT in a Go goroutine. It acts as the consumer in the SPSC (single-producer
// single-consumer) pattern, reading frames from the ringbuffer that the producer
// goroutine writes to.
//
// Real-time constraints:
// - Must be extremely fast (runs in real-time audio context)
// - Should avoid allocations
// - Cannot block or perform slow operations
// - Runs independently from Go's scheduler
func (fp *FramesPlayer) audioCallback(
	input, output []byte,
	frameCount uint,
	timeInfo *portaudio.StreamCallbackTimeInfo,
	statusFlags portaudio.StreamCallbackFlags,
) portaudio.StreamCallbackResult {

	bytesNeeded := int(frameCount) * fp.channels * fp.bytesPerSample
	bytesWritten := 0

	// Check if producer is done and buffer is empty
	if fp.producerDone.Load() && fp.ringbuf.AvailableRead() == 0 && fp.currentFrame == nil {
		fp.playbackComplete.Store(true)
		return portaudio.Complete
	}

	// Fill output buffer from AudioFrames
	for bytesWritten < bytesNeeded {
		// Get next frame if we don't have one
		if fp.currentFrame == nil {
			// Peek at available frames to check format before consuming
			if fp.ringbuf.AvailableRead() > 0 {
				frames, err := fp.ringbuf.Read(1)
				if err != nil || len(frames) == 0 {
					// No frames available, fill with silence
					break
				}

				// Check if format changed
				if int(frames[0].Format.SampleRate) != fp.sampleRate ||
					int(frames[0].Format.Channels) != fp.channels ||
					int(frames[0].Format.BitsPerSample) != fp.bitsPerSample {

					slog.Info("Audio format changed in callback, stopping stream",
						"old_rate", fp.sampleRate,
						"new_rate", frames[0].Format.SampleRate,
						"old_channels", fp.channels,
						"new_channels", frames[0].Format.Channels,
						"old_bits", fp.bitsPerSample,
						"new_bits", frames[0].Format.BitsPerSample)

					// Store the frame with new format for producer to handle
					fp.frameMu.Lock()
					fp.pendingFrame = &frames[0]
					fp.frameMu.Unlock()

					fp.formatChangePending.Store(true)
					return portaudio.Complete
				}

				fp.currentFrame = &frames[0]
				fp.frameOffset = 0
			} else {
				// No frames available, fill with silence
				break
			}
		}

		// Copy audio data from current frame
		remainingInFrame := len(fp.currentFrame.Audio) - fp.frameOffset
		remainingInOutput := bytesNeeded - bytesWritten

		bytesToCopy := min(remainingInFrame, remainingInOutput)

		copy(output[bytesWritten:bytesWritten+bytesToCopy],
			fp.currentFrame.Audio[fp.frameOffset:fp.frameOffset+bytesToCopy])

		bytesWritten += bytesToCopy
		fp.frameOffset += bytesToCopy

		// If we've consumed the entire frame, move to next
		if fp.frameOffset >= len(fp.currentFrame.Audio) {
			fp.currentFrame = nil
			fp.frameOffset = 0
		}
	}

	// Fill remainder with silence if needed
	if bytesWritten < bytesNeeded {
		clear(output[bytesWritten:bytesNeeded])
	}

	return portaudio.Continue
}

// producer reads from decoder and writes AudioFrames to ringbuffer
func (fp *FramesPlayer) producer() {
	defer fp.wg.Done()
	defer fp.producerDone.Store(true)

	bufferBytes := fp.samplesPerFrame * fp.channels * fp.bytesPerSample
	buffer := make([]byte, bufferBytes)

	slog.Info("Producer started",
		"samples_per_frame", fp.samplesPerFrame,
		"buffer_bytes", bufferBytes)

	totalFramesProduced := 0

	for {
		select {
		case <-fp.stopChan:
			slog.Info("Producer stopped", "total_frames", totalFramesProduced)
			return
		default:
		}

		// Check if format change is pending and handle it
		if fp.formatChangePending.Load() {
			fp.frameMu.Lock()
			newFrame := fp.pendingFrame
			fp.frameMu.Unlock()

			if newFrame != nil {
				slog.Info("Handling format change in producer")
				if err := fp.reinitializeStream(newFrame); err != nil {
					slog.Error("Failed to reinitialize stream", "error", err)
					return
				}

				// Set as current frame for playback
				fp.mu.Lock()
				fp.currentFrame = newFrame
				fp.frameOffset = 0
				fp.mu.Unlock()

				// Clear pending state
				fp.frameMu.Lock()
				fp.pendingFrame = nil
				fp.frameMu.Unlock()

				fp.formatChangePending.Store(false)
				slog.Info("Stream reinitialized with new format")

				// Update buffer size for new format
				bufferBytes = fp.samplesPerFrame * fp.channels * fp.bytesPerSample
				buffer = make([]byte, bufferBytes)
			}
		}

		// Decode samples
		samplesRead, err := fp.decoder.DecodeSamples(fp.samplesPerFrame, buffer)
		if err != nil || samplesRead == 0 {
			slog.Info("Producer finished",
				"error", err,
				"samples_read", samplesRead,
				"total_frames", totalFramesProduced)
			return
		}

		bytesToWrite := samplesRead * fp.channels * fp.bytesPerSample

		// Create AudioFrame with format metadata
		frame := audioframe.AudioFrame{
			Format: audioframe.FrameFormat{
				SampleRate:    uint32(fp.sampleRate),
				Channels:      uint8(fp.channels),
				BitsPerSample: uint8(fp.bitsPerSample),
			},
			SamplesCount: uint16(samplesRead),
			Audio:        make([]byte, bytesToWrite),
		}
		copy(frame.Audio, buffer[:bytesToWrite])

		// Write to ringbuffer - handles partial writes automatically
		// Retry until frame is written
		toWrite := []audioframe.AudioFrame{frame}
		for len(toWrite) > 0 {
			written, _ := fp.ringbuf.Write(toWrite)
			if written > 0 {
				totalFramesProduced += written
				toWrite = toWrite[written:]
			}

			// Check if stopped
			select {
			case <-fp.stopChan:
				return
			default:
			}

			// Yield to consumer if partial write or buffer full
			if len(toWrite) > 0 {
				// Small sleep to avoid busy waiting
			}
		}
	}
}

func (fp *FramesPlayer) Wait() {
	// First wait for producer to finish
	fp.wg.Wait()

	// Then wait for audio callback to finish playing all buffered audio
	// Poll until playbackComplete is set by the audio callback
	for !fp.playbackComplete.Load() {
		time.Sleep(10 * time.Millisecond)
	}
}

func (fp *FramesPlayer) Stop() error {
	fp.mu.Lock()
	if fp.stopped {
		fp.mu.Unlock()
		return nil
	}
	fp.stopped = true
	fp.mu.Unlock()

	close(fp.stopChan)
	fp.wg.Wait()

	if fp.stream != nil {
		if err := fp.stream.StopStream(); err != nil {
			slog.Warn("Failed to stop stream", "error", err)
		}
		if err := fp.stream.CloseCallback(); err != nil {
			slog.Warn("Failed to close stream", "error", err)
		}
	}

	if fp.decoder != nil {
		if err := fp.decoder.Close(); err != nil {
			slog.Warn("Failed to close decoder", "error", err)
		}
	}

	slog.Info("Playback stopped")
	return nil
}

func (fp *FramesPlayer) GetBufferStatus() (available, size uint64) {
	return fp.ringbuf.AvailableRead(), fp.ringbuf.Size()
}

func main() {
	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Parse flags
	deviceIdx := flag.Int("device", 1, "Audio output device index")
	bufferCapacity := flag.Uint64("capacity", 256, "Ringbuffer capacity (number of frames)")
	paFrames := flag.Int("paframes", 512, "PortAudio frames per buffer")
	samplesPerFrame := flag.Int("samples", 4096, "Samples per AudioFrame")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: framesplayer [options] <audio_file>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Plays audio using PortAudio callback mode with AudioFrameRingBuffer")
		fmt.Fprintln(os.Stderr, "Demonstrates frame-based buffering with format metadata")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  framesplayer music.mp3")
		fmt.Fprintln(os.Stderr, "  framesplayer -device 0 -v music.flac")
		fmt.Fprintln(os.Stderr, "  framesplayer -capacity 512 -samples 2048 music.wav")
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

	slog.Info("PortAudio initialized", "version", portaudio.GetVersion())
	slog.Info("Configuration",
		"device_index", *deviceIdx,
		"frame_capacity", *bufferCapacity,
		"pa_frames_per_buffer", *paFrames,
		"samples_per_audioframe", *samplesPerFrame)

	// Create frame-based player
	player := NewFramesPlayer(*deviceIdx, *bufferCapacity, *paFrames, *samplesPerFrame)

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

	// Monitor buffer status (only in verbose mode)
	if *verbose {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					available, size := player.GetBufferStatus()
					slog.Debug("Buffer status",
						"available_frames", available,
						"capacity", size,
						"fill_percent", fmt.Sprintf("%.1f%%", float64(available)/float64(size)*100))
				case <-sigChan:
					return
				case <-player.stopChan:
					return
				}
			}
		}()
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
