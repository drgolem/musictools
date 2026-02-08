package ringbuffer

import (
	"sync/atomic"

	"learnRingbuffer/pkg/types"
)

// Re-export common ringbuffer errors for backwards compatibility
var (
	ErrInsufficientSpace = types.ErrInsufficientSpace
	ErrInsufficientData  = types.ErrInsufficientData
)

// RingBuffer is a lock-free single-producer single-consumer ring buffer
// optimized for audio applications with byte data.
//
// RingBuffer implements io.Reader and io.Writer interfaces, making it compatible
// with the standard Go I/O ecosystem. However, note the thread safety requirements:
//   - Write() must only be called by the producer thread (implements io.Writer)
//   - Read() must only be called by the consumer thread (implements io.Reader)
type RingBuffer struct {
	buffer   []byte
	size     uint64 // must be power of 2
	mask     uint64 // size - 1, for efficient modulo
	writePos atomic.Uint64
	readPos  atomic.Uint64
}

// New creates a new ring buffer with the given size.
// Size will be rounded up to the next power of 2 for efficiency.
func New(size uint64) *RingBuffer {
	// Round up to next power of 2
	size = nextPowerOf2(size)

	return &RingBuffer{
		buffer: make([]byte, size),
		size:   size,
		mask:   size - 1,
	}
}

// Write writes data to the ring buffer, implementing io.Writer.
// It writes all of len(data) bytes or returns an error.
//
// Unlike some io.Writer implementations, this method does not perform partial writes.
// It will either write all data successfully or return ErrInsufficientSpace without
// writing any data.
//
// This method must only be called by the producer thread.
func (rb *RingBuffer) Write(data []byte) (int, error) {
	dataLen := uint64(len(data))
	if dataLen == 0 {
		return 0, nil
	}

	available := rb.AvailableWrite()
	if dataLen > available {
		return 0, ErrInsufficientSpace
	}

	writePos := rb.writePos.Load()

	// Calculate the actual position in the buffer
	start := writePos & rb.mask
	end := (writePos + dataLen) & rb.mask

	if end > start {
		// Single contiguous write
		copy(rb.buffer[start:end], data)
	} else {
		// Write wraps around the buffer
		firstChunk := rb.size - start
		copy(rb.buffer[start:], data[:firstChunk])
		copy(rb.buffer[:end], data[firstChunk:])
	}

	// Atomic update of write position
	rb.writePos.Store(writePos + dataLen)

	return int(dataLen), nil
}

// Read reads up to len(data) bytes from the ring buffer into data, implementing io.Reader.
//
// Read will read as many bytes as are available, up to len(data). If fewer bytes are
// available than requested, it reads what's available and returns the count without error.
// If the buffer is empty, it returns (0, ErrInsufficientData).
//
// This follows io.Reader semantics where ErrInsufficientData is analogous to io.EOF,
// indicating no data is currently available for reading.
//
// This method must only be called by the consumer thread.
func (rb *RingBuffer) Read(data []byte) (int, error) {
	dataLen := uint64(len(data))
	if dataLen == 0 {
		return 0, nil
	}

	available := rb.AvailableRead()
	if available == 0 {
		return 0, ErrInsufficientData
	}

	// Read only what's available
	toRead := min(dataLen, available)

	readPos := rb.readPos.Load()

	// Calculate the actual position in the buffer
	start := readPos & rb.mask
	end := (readPos + toRead) & rb.mask

	if end > start {
		// Single contiguous read
		copy(data[:toRead], rb.buffer[start:end])
	} else {
		// Read wraps around the buffer
		firstChunk := rb.size - start
		copy(data[:firstChunk], rb.buffer[start:])
		copy(data[firstChunk:toRead], rb.buffer[:end])
	}

	// Atomic update of read position
	rb.readPos.Store(readPos + toRead)

	return int(toRead), nil
}

// AvailableWrite returns the number of bytes available for writing
func (rb *RingBuffer) AvailableWrite() uint64 {
	writePos := rb.writePos.Load()
	readPos := rb.readPos.Load()
	return rb.size - (writePos - readPos)
}

// AvailableRead returns the number of bytes available for reading
func (rb *RingBuffer) AvailableRead() uint64 {
	writePos := rb.writePos.Load()
	readPos := rb.readPos.Load()
	return writePos - readPos
}

// Size returns the total size of the ring buffer
func (rb *RingBuffer) Size() uint64 {
	return rb.size
}

// ReadSlices returns one or two slices that provide zero-copy access to the available data.
// The data may be split into two slices if it wraps around the ring buffer.
// After processing the data, call Consume() to advance the read position.
// This should only be called by the consumer thread.
//
// Returns:
//   - first: The first (or only) slice of available data
//   - second: The second slice if data wraps around, nil otherwise
//   - total: Total number of bytes available across both slices
func (rb *RingBuffer) ReadSlices() (first, second []byte, total uint64) {
	available := rb.AvailableRead()
	if available == 0 {
		return nil, nil, 0
	}

	readPos := rb.readPos.Load()
	start := readPos & rb.mask
	end := (readPos + available) & rb.mask

	if end > start {
		// Data is contiguous
		return rb.buffer[start:end], nil, available
	}

	// Data wraps around
	firstChunk := rb.buffer[start:]
	secondChunk := rb.buffer[:end]
	return firstChunk, secondChunk, available
}

// PeekContiguous returns a slice providing zero-copy access to the contiguous
// portion of available data. This may be less than the total available data
// if the data wraps around the buffer.
// After processing, call Consume() to advance the read position.
// This should only be called by the consumer thread.
func (rb *RingBuffer) PeekContiguous() []byte {
	available := rb.AvailableRead()
	if available == 0 {
		return nil
	}

	readPos := rb.readPos.Load()
	start := readPos & rb.mask
	end := (readPos + available) & rb.mask

	if end > start {
		// All data is contiguous
		return rb.buffer[start:end]
	}

	// Data wraps around, return only the first contiguous chunk
	return rb.buffer[start:]
}

// Consume advances the read position by n bytes without copying data.
// This is used in conjunction with ReadSlices() or PeekContiguous() for zero-copy reads.
// Returns an error if trying to consume more bytes than are available.
// This should only be called by the consumer thread.
func (rb *RingBuffer) Consume(n uint64) error {
	if n == 0 {
		return nil
	}

	available := rb.AvailableRead()
	if n > available {
		return ErrInsufficientData
	}

	readPos := rb.readPos.Load()
	rb.readPos.Store(readPos + n)
	return nil
}

// Reset clears the ring buffer by resetting read and write positions
func (rb *RingBuffer) Reset() {
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
