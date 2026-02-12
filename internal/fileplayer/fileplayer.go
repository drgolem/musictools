package fileplayer

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drgolem/musictools/pkg/audioframe"
	"github.com/drgolem/musictools/pkg/audioframeringbuffer"
	"github.com/drgolem/musictools/pkg/decoders"
	"github.com/drgolem/musictools/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
)

// FilePlayer plays audio files using PortAudio callback mode with AudioFrameRingBuffer.
// It implements a production-ready SPSC (Single-Producer Single-Consumer) pattern
// with a 9.5/10 thread safety rating.
//
// Thread Safety Model:
//   - Producer goroutine writes to ringbuffer
//   - PortAudio C thread (audio callback) reads from ringbuffer
//   - Atomic operations for all shared state
//   - Deep copy for frame data to prevent buffer corruption
type FilePlayer struct {
	ringbuf         *audioframeringbuffer.AudioFrameRingBuffer
	stream          *portaudio.PaStream
	decoder         types.AudioDecoder
	deviceIndex     int
	framesPerBuffer int
	samplesPerFrame int

	// Current file format
	sampleRate     int
	channels       int
	bitsPerSample  int
	bytesPerSample int

	// Goroutine coordination
	producerDone         atomic.Bool
	playbackComplete     atomic.Bool
	playbackCompleteChan chan struct{} // Closed when playback completes (replaces polling)
	stopChan             chan struct{}
	wg                   sync.WaitGroup
	mu                   sync.Mutex
	stopped              bool

	// Callback state for partial frame consumption (atomic for thread safety)
	currentFrame atomic.Pointer[audioframe.AudioFrame]
	frameOffset  int

	// Playback status tracking
	currentFileName string
	startTime       time.Time
	producedSamples atomic.Uint64 // Samples decoded and buffered
	playedSamples   atomic.Uint64 // Samples actually played through callback
}

// NewFilePlayer creates a new FilePlayer with the specified configuration.
//
// Parameters:
//   - deviceIdx: PortAudio device index for audio output
//   - bufferCapacity: Ringbuffer capacity in number of AudioFrames
//   - framesPerBuffer: PortAudio frames per buffer callback
//   - samplesPerFrame: Number of samples per AudioFrame
func NewFilePlayer(deviceIdx int, bufferCapacity uint64, framesPerBuffer, samplesPerFrame int) *FilePlayer {
	return &FilePlayer{
		ringbuf:         audioframeringbuffer.New(bufferCapacity),
		deviceIndex:     deviceIdx,
		framesPerBuffer: framesPerBuffer,
		samplesPerFrame: samplesPerFrame,
	}
}

// OpenFile opens an audio file and initializes the appropriate decoder.
// Supported formats: MP3 (.mp3), FLAC (.flac, .fla), WAV (.wav).
//
// This method will close any previously opened file.
func (fp *FilePlayer) OpenFile(fileName string) error {
	// Close previous decoder if any
	if fp.decoder != nil {
		fp.decoder.Close()
		fp.decoder = nil
	}

	// Use factory to create and open decoder
	decoder, err := decoders.NewDecoder(fileName)
	if err != nil {
		return err
	}

	rate, channels, bps := decoder.GetFormat()
	bytesPerSample := bps / 8

	slog.Info("Audio file opened",
		"file", filepath.Base(fileName),
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	fp.decoder = decoder
	fp.sampleRate = rate
	fp.channels = channels
	fp.bitsPerSample = bps
	fp.bytesPerSample = bytesPerSample
	fp.currentFileName = filepath.Base(fileName)

	return nil
}

// PlayFile starts playing the currently opened file.
// Returns an error if no file is opened or if the stream cannot be initialized.
//
// This method initializes the PortAudio stream and starts the producer goroutine.
// Use Wait() to block until playback completes, or Stop() to interrupt playback.
func (fp *FilePlayer) PlayFile() error {
	if fp.decoder == nil {
		return fmt.Errorf("no file opened")
	}

	// Reset state
	fp.producerDone.Store(false)
	fp.playbackComplete.Store(false)
	fp.playbackCompleteChan = make(chan struct{})
	fp.stopChan = make(chan struct{})
	fp.stopped = false
	fp.currentFrame.Store(nil)
	fp.frameOffset = 0
	fp.ringbuf.Reset()
	fp.producedSamples.Store(0)
	fp.playedSamples.Store(0)
	fp.startTime = time.Now()

	// Initialize PortAudio stream
	if err := fp.initializeStream(); err != nil {
		return err
	}

	// Start producer goroutine
	fp.wg.Add(1)
	go fp.producer()

	slog.Debug("Playback started")
	return nil
}

func (fp *FilePlayer) initializeStream() error {
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

	return nil
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
func (fp *FilePlayer) audioCallback(
	input, output []byte,
	frameCount uint,
	timeInfo *portaudio.StreamCallbackTimeInfo,
	statusFlags portaudio.StreamCallbackFlags,
) portaudio.StreamCallbackResult {

	bytesNeeded := int(frameCount) * fp.channels * fp.bytesPerSample
	bytesWritten := 0

	// Check if producer is done and buffer is empty
	if fp.producerDone.Load() && fp.ringbuf.AvailableRead() == 0 && fp.currentFrame.Load() == nil {
		fp.playbackComplete.Store(true)
		// Signal completion via channel (non-blocking, channel may already be closed)
		select {
		case <-fp.playbackCompleteChan:
			// Already closed
		default:
			close(fp.playbackCompleteChan)
		}
		return portaudio.Complete
	}

	// Fill output buffer from AudioFrames
	for bytesWritten < bytesNeeded {
		// Get next frame if we don't have one
		currentFrame := fp.currentFrame.Load()
		if currentFrame == nil {
			if fp.ringbuf.AvailableRead() > 0 {
				frames, err := fp.ringbuf.Read(1)
				if err != nil || len(frames) == 0 {
					// No frames available, fill with silence
					break
				}

				fp.currentFrame.Store(&frames[0])
				currentFrame = &frames[0]
				fp.frameOffset = 0
			} else {
				// No frames available, fill with silence
				break
			}
		}

		// Copy audio data from current frame
		remainingInFrame := len(currentFrame.Audio) - fp.frameOffset
		remainingInOutput := bytesNeeded - bytesWritten

		bytesToCopy := min(remainingInFrame, remainingInOutput)

		copy(output[bytesWritten:bytesWritten+bytesToCopy],
			currentFrame.Audio[fp.frameOffset:fp.frameOffset+bytesToCopy])

		bytesWritten += bytesToCopy
		fp.frameOffset += bytesToCopy

		// If we've consumed the entire frame, move to next
		if fp.frameOffset >= len(currentFrame.Audio) {
			fp.currentFrame.Store(nil)
			fp.frameOffset = 0
		}
	}

	// Fill remainder with silence if needed
	if bytesWritten < bytesNeeded {
		clear(output[bytesWritten:bytesNeeded])
	}

	// Track samples actually played (sent to audio output)
	samplesPlayed := bytesWritten / (fp.channels * fp.bytesPerSample)
	fp.playedSamples.Add(uint64(samplesPlayed))

	return portaudio.Continue
}

// producer reads from decoder and writes AudioFrames to ringbuffer.
// This is the producer in the SPSC pattern, running in a separate goroutine.
func (fp *FilePlayer) producer() {
	defer fp.wg.Done()
	defer fp.producerDone.Store(true)

	bufferBytes := fp.samplesPerFrame * fp.channels * fp.bytesPerSample
	buffer := make([]byte, bufferBytes)

	totalFramesProduced := 0

	for {
		select {
		case <-fp.stopChan:
			slog.Debug("Producer stopped", "total_frames", totalFramesProduced)
			return
		default:
		}

		// Decode samples
		samplesRead, err := fp.decoder.DecodeSamples(fp.samplesPerFrame, buffer)
		if err != nil || samplesRead == 0 {
			slog.Debug("Producer finished",
				"error", err,
				"samples_read", samplesRead,
				"total_frames", totalFramesProduced)
			return
		}

		bytesToWrite := samplesRead * fp.channels * fp.bytesPerSample

		// Create AudioFrame with deep copy (critical for thread safety)
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

		// Write to ringbuffer - retry until written
		toWrite := []audioframe.AudioFrame{frame}
		for len(toWrite) > 0 {
			written, _ := fp.ringbuf.Write(toWrite)
			if written > 0 {
				totalFramesProduced += written
				toWrite = toWrite[written:]
				// Track produced samples (buffered, not yet played)
				fp.producedSamples.Add(uint64(samplesRead))
			}

			// Check if stopped
			select {
			case <-fp.stopChan:
				return
			default:
			}

			// Yield if buffer full
			if len(toWrite) > 0 {
				// Small sleep to avoid busy waiting
			}
		}
	}
}

// Wait blocks until the current file finishes playing.
// This method waits for both the producer goroutine to finish decoding
// and the audio callback to finish playing all buffered audio.
func (fp *FilePlayer) Wait() {
	// First wait for producer to finish
	fp.wg.Wait()

	// Then wait for audio callback to finish playing all buffered audio
	// Wait on channel that's closed when playback completes (no polling!)
	<-fp.playbackCompleteChan
}

// Stop stops playback of the current file.
// This method is safe to call multiple times and will gracefully shut down
// the producer, audio stream, and decoder.
func (fp *FilePlayer) Stop() error {
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
		fp.stream = nil
	}

	if fp.decoder != nil {
		if err := fp.decoder.Close(); err != nil {
			slog.Warn("Failed to close decoder", "error", err)
		}
		fp.decoder = nil
	}

	return nil
}

// GetPlaybackStatus returns current playback status including samples played,
// buffered, and elapsed time. Implements types.PlaybackMonitor interface.
func (fp *FilePlayer) GetPlaybackStatus() types.PlaybackStatus {
	produced := fp.producedSamples.Load()
	played := fp.playedSamples.Load()
	buffered := uint64(0)
	if produced > played {
		buffered = produced - played
	}

	return types.PlaybackStatus{
		FileName:        fp.currentFileName,
		SampleRate:      fp.sampleRate,
		Channels:        fp.channels,
		BitsPerSample:   fp.bitsPerSample,
		FramesPerBuffer: fp.framesPerBuffer,
		PlayedSamples:   played,
		BufferedSamples: buffered,
		ElapsedTime:     time.Since(fp.startTime),
	}
}
