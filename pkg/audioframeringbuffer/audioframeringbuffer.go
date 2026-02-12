package audioframeringbuffer

import (
	"sync/atomic"

	"github.com/drgolem/musictools/pkg/audioframe"
	"github.com/drgolem/musictools/pkg/types"
)

// Re-export common ringbuffer errors for backwards compatibility
var (
	ErrInsufficientSpace = types.ErrInsufficientSpace
	ErrInsufficientData  = types.ErrInsufficientData
)

// AudioFrameRingBuffer is a lock-free single-producer single-consumer ring buffer
// for AudioFrame objects, optimized for audio streaming applications.
//
// Thread safety:
//   - Write() must only be called by the producer thread
//   - Read() must only be called by the consumer thread
//
// The buffer capacity is automatically rounded up to the next power of 2 for
// efficient modulo operations using bitwise AND.
type AudioFrameRingBuffer struct {
	buffer   []audioframe.AudioFrame
	size     uint64 // must be power of 2
	mask     uint64 // size - 1, for efficient modulo
	writePos atomic.Uint64
	readPos  atomic.Uint64
}

// New creates a new AudioFrame ring buffer with the given capacity (number of frames).
// Capacity will be rounded up to the next power of 2 for efficiency.
//
// Example:
//
//	rb := audioframeringbuffer.New(1024) // Creates buffer for 1024 frames
func New(capacity uint64) *AudioFrameRingBuffer {
	// Round up to next power of 2
	capacity = nextPowerOf2(capacity)

	return &AudioFrameRingBuffer{
		buffer: make([]audioframe.AudioFrame, capacity),
		size:   capacity,
		mask:   capacity - 1,
	}
}

// Write writes AudioFrames to the ring buffer.
// It writes as many frames as possible and returns the number of frames written.
// This allows partial writes, similar to io.Writer pattern.
//
// This method must only be called by the producer thread.
//
// The Audio slice is deep copied, so callers may safely reuse the frame buffers
// after Write returns without corrupting data in the ring buffer.
//
// Returns:
//   - int: number of frames actually written (may be less than requested)
//   - error: ErrInsufficientSpace if no space available (0 frames written), nil otherwise
//
// Example:
//
//	frames := []audioframe.AudioFrame{frame1, frame2, frame3}
//	written, err := rb.Write(frames)
//	if written < len(frames) {
//	    // Handle partial write - could retry later with frames[written:]
//	}
func (rb *AudioFrameRingBuffer) Write(frames []audioframe.AudioFrame) (int, error) {
	frameCount := uint64(len(frames))
	if frameCount == 0 {
		return 0, nil
	}

	available := rb.AvailableWrite()
	toWrite := min(frameCount, available)

	if toWrite == 0 {
		return 0, ErrInsufficientSpace
	}

	writePos := rb.writePos.Load()

	// Write frames to the buffer (copy by value, including deep copy of Audio slice)
	for i := uint64(0); i < toWrite; i++ {
		pos := (writePos + i) & rb.mask
		rb.buffer[pos] = frames[i]
		// Deep copy the Audio slice to prevent data corruption if caller reuses buffer
		rb.buffer[pos].Audio = make([]byte, len(frames[i].Audio))
		copy(rb.buffer[pos].Audio, frames[i].Audio)
	}

	// Atomic update of write position
	rb.writePos.Store(writePos + toWrite)

	return int(toWrite), nil
}

// Read reads up to numFrames from the ring buffer.
// Returns a slice of frames (up to numFrames requested) and an error if buffer is empty.
//
// If fewer frames are available than requested, returns what's available without error.
// If the buffer is empty, returns (nil, ErrInsufficientData).
//
// This method must only be called by the consumer thread.
//
// Returns:
//   - []AudioFrame: slice containing the frames read (may be fewer than requested)
//   - error: ErrInsufficientData if buffer is empty, nil otherwise
//
// Example:
//
//	frames, err := rb.Read(10) // Request up to 10 frames
//	if err != nil {
//	    // Handle empty buffer
//	}
//	// Process frames (may be fewer than 10)
func (rb *AudioFrameRingBuffer) Read(numFrames int) ([]audioframe.AudioFrame, error) {
	if numFrames <= 0 {
		return nil, nil
	}

	available := rb.AvailableRead()
	if available == 0 {
		return nil, ErrInsufficientData
	}

	// Read only what's available
	toRead := min(uint64(numFrames), available)

	readPos := rb.readPos.Load()
	result := make([]audioframe.AudioFrame, toRead)

	// Read frames from the buffer (copy by value)
	for i := uint64(0); i < toRead; i++ {
		pos := (readPos + i) & rb.mask
		result[i] = rb.buffer[pos]
	}

	// Atomic update of read position
	rb.readPos.Store(readPos + toRead)

	return result, nil
}

// AvailableWrite returns the number of frames available for writing
func (rb *AudioFrameRingBuffer) AvailableWrite() uint64 {
	writePos := rb.writePos.Load()
	readPos := rb.readPos.Load()
	return rb.size - (writePos - readPos)
}

// AvailableRead returns the number of frames available for reading
func (rb *AudioFrameRingBuffer) AvailableRead() uint64 {
	writePos := rb.writePos.Load()
	readPos := rb.readPos.Load()
	return writePos - readPos
}

// Size returns the total capacity of the ring buffer (number of frames)
func (rb *AudioFrameRingBuffer) Size() uint64 {
	return rb.size
}

// Reset clears the ring buffer by resetting read and write positions.
// This does not zero out the buffer memory, just resets the position counters.
func (rb *AudioFrameRingBuffer) Reset() {
	rb.readPos.Store(0)
	rb.writePos.Store(0)
}

// nextPowerOf2 rounds up to the next power of 2
func nextPowerOf2(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return n
}
