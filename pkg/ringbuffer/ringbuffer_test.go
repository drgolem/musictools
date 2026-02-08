package ringbuffer

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Test that size is rounded up to power of 2
	tests := []struct {
		input    uint64
		expected uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{7, 8},
		{100, 128},
		{1024, 1024},
		{1025, 2048},
	}

	for _, tt := range tests {
		rb := New(tt.input)
		if rb.Size() != tt.expected {
			t.Errorf("New(%d): expected size %d, got %d", tt.input, tt.expected, rb.Size())
		}
	}
}

func TestWriteRead(t *testing.T) {
	rb := New(16)

	// Write some data
	writeData := []byte("hello")
	n, err := rb.Write(writeData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Write: expected %d bytes, wrote %d", len(writeData), n)
	}

	// Check available data
	if rb.AvailableRead() != uint64(len(writeData)) {
		t.Errorf("AvailableRead: expected %d, got %d", len(writeData), rb.AvailableRead())
	}

	// Read the data
	readData := make([]byte, 10)
	n, err = rb.Read(readData)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Read: expected %d bytes, read %d", len(writeData), n)
	}
	if !bytes.Equal(readData[:n], writeData) {
		t.Errorf("Read: expected %s, got %s", writeData, readData[:n])
	}
}

func TestWrapAround(t *testing.T) {
	rb := New(8)

	// Fill buffer partially
	data1 := []byte("abc")
	rb.Write(data1)

	// Read it
	readBuf := make([]byte, 3)
	rb.Read(readBuf)

	// Write more data that will wrap around
	data2 := []byte("defgh")
	n, err := rb.Write(data2)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data2) {
		t.Errorf("Write: expected %d bytes, wrote %d", len(data2), n)
	}

	// Read all
	readBuf2 := make([]byte, 10)
	n, err = rb.Read(readBuf2)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !bytes.Equal(readBuf2[:n], data2) {
		t.Errorf("Read after wrap: expected %s, got %s", data2, readBuf2[:n])
	}
}

func TestInsufficientSpace(t *testing.T) {
	rb := New(8)

	// Try to write more than capacity
	data := make([]byte, 10)
	_, err := rb.Write(data)
	if err != ErrInsufficientSpace {
		t.Errorf("Write: expected ErrInsufficientSpace, got %v", err)
	}

	// Write up to capacity
	data2 := make([]byte, 8)
	n, err := rb.Write(data2)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 8 {
		t.Errorf("Write: expected 8 bytes, wrote %d", n)
	}

	// Try to write when full
	_, err = rb.Write([]byte{1})
	if err != ErrInsufficientSpace {
		t.Errorf("Write to full buffer: expected ErrInsufficientSpace, got %v", err)
	}
}

func TestInsufficientData(t *testing.T) {
	rb := New(16)

	// Try to read from empty buffer
	readBuf := make([]byte, 5)
	_, err := rb.Read(readBuf)
	if err != ErrInsufficientData {
		t.Errorf("Read from empty buffer: expected ErrInsufficientData, got %v", err)
	}

	// Write some data
	rb.Write([]byte("hi"))

	// Read more than available (should read what's available)
	readBuf2 := make([]byte, 10)
	n, err := rb.Read(readBuf2)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 2 {
		t.Errorf("Read: expected 2 bytes, read %d", n)
	}
}

func TestReset(t *testing.T) {
	rb := New(16)

	// Write and read some data
	rb.Write([]byte("test"))
	readBuf := make([]byte, 2)
	rb.Read(readBuf)

	// Reset
	rb.Reset()

	if rb.AvailableRead() != 0 {
		t.Errorf("After reset: expected 0 bytes available, got %d", rb.AvailableRead())
	}
	if rb.AvailableWrite() != rb.Size() {
		t.Errorf("After reset: expected %d bytes writable, got %d", rb.Size(), rb.AvailableWrite())
	}
}

func TestConcurrentProducerConsumer(t *testing.T) {
	rb := New(1024)

	const iterations = 10000
	const chunkSize = 32

	var wg sync.WaitGroup
	wg.Add(2)

	errors := make(chan error, 2)

	// Producer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			// Create test data
			data := make([]byte, chunkSize)
			for j := 0; j < chunkSize; j++ {
				data[j] = byte(i % 256)
			}

			// Retry write if buffer is full
			for {
				_, err := rb.Write(data)
				if err == nil {
					break
				}
				if err != ErrInsufficientSpace {
					errors <- err
					return
				}
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Consumer goroutine
	go func() {
		defer wg.Done()
		totalRead := 0
		for totalRead < iterations*chunkSize {
			readBuf := make([]byte, chunkSize)
			n, err := rb.Read(readBuf)
			if err == ErrInsufficientData {
				time.Sleep(time.Microsecond)
				continue
			}
			if err != nil {
				errors <- err
				return
			}

			// Verify data
			expectedVal := byte((totalRead / chunkSize) % 256)
			for j := 0; j < n; j++ {
				if readBuf[j] != expectedVal {
					t.Errorf("Data corruption at byte %d: expected %d, got %d",
						totalRead+j, expectedVal, readBuf[j])
					return
				}
			}

			totalRead += n
		}
	}()

	// Wait for completion with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case err := <-errors:
		t.Fatalf("Error during concurrent test: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("Test timeout - possible deadlock")
	}
}

func TestAudioSimulation(t *testing.T) {
	// Simulate audio buffer scenario
	// 44.1kHz, 16-bit stereo = 176,400 bytes/sec
	// Buffer for ~100ms = 17,640 bytes, round up to 32KB
	rb := New(32 * 1024)

	const sampleRate = 44100
	const channels = 2
	const bytesPerSample = 2
	const bufferMs = 10 // 10ms chunks

	chunkSize := (sampleRate * channels * bytesPerSample * bufferMs) / 1000

	var wg sync.WaitGroup
	wg.Add(2)

	// Simulated audio producer
	go func() {
		defer wg.Done()
		chunk := make([]byte, chunkSize)
		for i := 0; i < 100; i++ { // 1 second of audio
			// Simulate audio data generation
			for j := range chunk {
				chunk[j] = byte(i + j)
			}

			// Wait for space
			for rb.AvailableWrite() < uint64(chunkSize) {
				time.Sleep(time.Millisecond)
			}

			rb.Write(chunk)
			time.Sleep(time.Duration(bufferMs) * time.Millisecond)
		}
	}()

	// Simulated audio consumer
	go func() {
		defer wg.Done()
		chunk := make([]byte, chunkSize)
		for i := 0; i < 100; i++ {
			// Wait for data
			for rb.AvailableRead() < uint64(chunkSize) {
				time.Sleep(time.Millisecond)
			}

			n, err := rb.Read(chunk)
			if err != nil && err != ErrInsufficientData {
				t.Errorf("Read error: %v", err)
				return
			}
			if n < chunkSize {
				t.Errorf("Underrun: expected %d bytes, got %d", chunkSize, n)
			}

			time.Sleep(time.Duration(bufferMs) * time.Millisecond)
		}
	}()

	wg.Wait()
}

func BenchmarkWrite(b *testing.B) {
	rb := New(64 * 1024)
	data := make([]byte, 256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rb.AvailableWrite() < uint64(len(data)) {
			rb.Reset()
		}
		rb.Write(data)
	}
}

func BenchmarkRead(b *testing.B) {
	rb := New(64 * 1024)
	data := make([]byte, 256)

	// Fill buffer
	for rb.AvailableWrite() >= uint64(len(data)) {
		rb.Write(data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rb.AvailableRead() < uint64(len(data)) {
			rb.Reset()
			for rb.AvailableWrite() >= uint64(len(data)) {
				rb.Write(data)
			}
		}
		rb.Read(data)
	}
}

func BenchmarkConcurrentReadWrite(b *testing.B) {
	rb := New(64 * 1024)
	data := make([]byte, 256)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Try write
			rb.Write(data)
			// Try read
			rb.Read(data)
		}
	})
}

func TestReadSlicesContiguous(t *testing.T) {
	rb := New(16)

	// Write some data that doesn't wrap
	data := []byte("hello")
	rb.Write(data)

	// Read slices
	first, second, total := rb.ReadSlices()
	if total != uint64(len(data)) {
		t.Errorf("ReadSlices: expected %d bytes, got %d", len(data), total)
	}
	if second != nil {
		t.Error("ReadSlices: expected no second slice for contiguous data")
	}
	if !bytes.Equal(first, data) {
		t.Errorf("ReadSlices: expected %s, got %s", data, first)
	}

	// Consume the data
	err := rb.Consume(total)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}

	// Verify buffer is empty
	if rb.AvailableRead() != 0 {
		t.Errorf("After consume: expected 0 bytes, got %d", rb.AvailableRead())
	}
}

func TestReadSlicesWrapped(t *testing.T) {
	rb := New(8)

	// Fill and partially drain to position pointers
	rb.Write([]byte("abc"))
	buf := make([]byte, 3)
	rb.Read(buf)

	// Write data that wraps around
	data := []byte("defgh")
	rb.Write(data)

	// Read slices - should get two slices
	first, second, total := rb.ReadSlices()
	if total != uint64(len(data)) {
		t.Errorf("ReadSlices: expected %d bytes, got %d", len(data), total)
	}
	if second == nil {
		t.Error("ReadSlices: expected second slice for wrapped data")
	}

	// Verify data integrity
	combined := append(first, second...)
	if !bytes.Equal(combined, data) {
		t.Errorf("ReadSlices: expected %s, got %s", data, combined)
	}

	// Consume all data
	err := rb.Consume(total)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if rb.AvailableRead() != 0 {
		t.Errorf("After consume: expected 0 bytes, got %d", rb.AvailableRead())
	}
}

func TestPeekContiguous(t *testing.T) {
	rb := New(16)

	// Write some data
	data := []byte("hello world")
	rb.Write(data)

	// Peek at it
	peeked := rb.PeekContiguous()
	if !bytes.Equal(peeked, data) {
		t.Errorf("PeekContiguous: expected %s, got %s", data, peeked)
	}

	// Verify data is still available (peek doesn't consume)
	if rb.AvailableRead() != uint64(len(data)) {
		t.Errorf("After peek: expected %d bytes available, got %d", len(data), rb.AvailableRead())
	}

	// Now consume it
	rb.Consume(uint64(len(peeked)))
	if rb.AvailableRead() != 0 {
		t.Errorf("After consume: expected 0 bytes, got %d", rb.AvailableRead())
	}
}

func TestPeekContiguousWrapped(t *testing.T) {
	rb := New(8)

	// Position pointers to cause wrap
	rb.Write([]byte("abcd"))
	buf := make([]byte, 4)
	rb.Read(buf)

	// Write data that wraps (6 bytes won't fit contiguously from position 4)
	data := []byte("efghij")
	rb.Write(data)

	// Peek should return only the contiguous first part (positions 4-7 = 4 bytes)
	peeked := rb.PeekContiguous()
	if len(peeked) >= len(data) {
		t.Errorf("PeekContiguous on wrapped data: expected less than %d bytes, got %d",
			len(data), len(peeked))
	}

	// Verify we got the first part
	expectedFirst := data[:len(peeked)]
	if !bytes.Equal(peeked, expectedFirst) {
		t.Errorf("PeekContiguous: expected %s, got %s", expectedFirst, peeked)
	}

	// Verify the first part is indeed 4 bytes (from position 4 to 7)
	if len(peeked) != 4 {
		t.Errorf("PeekContiguous: expected 4 contiguous bytes before wrap, got %d", len(peeked))
	}
}

func TestConsumeError(t *testing.T) {
	rb := New(16)

	// Write some data
	rb.Write([]byte("hello"))

	// Try to consume more than available
	err := rb.Consume(100)
	if err != ErrInsufficientData {
		t.Errorf("Consume: expected ErrInsufficientData, got %v", err)
	}

	// Verify nothing was consumed
	if rb.AvailableRead() != 5 {
		t.Errorf("After failed consume: expected 5 bytes, got %d", rb.AvailableRead())
	}
}

func TestZeroCopyPartialConsume(t *testing.T) {
	rb := New(16)

	// Write data
	data := []byte("hello world")
	rb.Write(data)

	// Peek and consume partially
	_ = rb.PeekContiguous()
	consumeSize := uint64(5) // Just "hello"

	err := rb.Consume(consumeSize)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}

	// Should have " world" remaining
	remaining := rb.AvailableRead()
	expected := uint64(len(data)) - consumeSize
	if remaining != expected {
		t.Errorf("After partial consume: expected %d bytes, got %d",
			expected, remaining)
	}

	// Read the rest normally
	buf := make([]byte, 10)
	n, _ := rb.Read(buf)
	if !bytes.Equal(buf[:n], []byte(" world")) {
		t.Errorf("Remaining data: expected ' world', got %s", buf[:n])
	}
}

func TestZeroCopyEmpty(t *testing.T) {
	rb := New(16)

	// Test ReadSlices on empty buffer
	first, second, total := rb.ReadSlices()
	if first != nil || second != nil || total != 0 {
		t.Error("ReadSlices on empty buffer should return nil slices and 0 total")
	}

	// Test PeekContiguous on empty buffer
	peeked := rb.PeekContiguous()
	if peeked != nil {
		t.Error("PeekContiguous on empty buffer should return nil")
	}

	// Test Consume(0)
	err := rb.Consume(0)
	if err != nil {
		t.Errorf("Consume(0) should not error, got %v", err)
	}
}

func BenchmarkZeroCopyRead(b *testing.B) {
	rb := New(64 * 1024)
	data := make([]byte, 256)

	// Fill buffer
	for rb.AvailableWrite() >= uint64(len(data)) {
		rb.Write(data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rb.AvailableRead() < uint64(len(data)) {
			rb.Reset()
			for rb.AvailableWrite() >= uint64(len(data)) {
				rb.Write(data)
			}
		}
		first, second, total := rb.ReadSlices()
		_ = first
		_ = second
		rb.Consume(min(total, uint64(len(data))))
	}
}

func BenchmarkPeekContiguous(b *testing.B) {
	rb := New(64 * 1024)
	data := make([]byte, 256)

	// Fill buffer
	for rb.AvailableWrite() >= uint64(len(data)) {
		rb.Write(data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rb.AvailableRead() < uint64(len(data)) {
			rb.Reset()
			for rb.AvailableWrite() >= uint64(len(data)) {
				rb.Write(data)
			}
		}
		peeked := rb.PeekContiguous()
		rb.Consume(min(uint64(len(peeked)), uint64(len(data))))
	}
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// Test that RingBuffer implements io.Reader and io.Writer
func TestIOInterfaces(t *testing.T) {
	rb := New(256)

	// Verify it implements io.Writer
	var _ io.Writer = rb

	// Verify it implements io.Reader
	var _ io.Reader = rb

	// Test writing via io.Writer interface
	var w io.Writer = rb
	data := []byte("Hello, io.Writer!")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("io.Writer.Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("io.Writer.Write: expected %d bytes, wrote %d", len(data), n)
	}

	// Test reading via io.Reader interface
	var r io.Reader = rb
	buffer := make([]byte, 50)
	n, err = r.Read(buffer)
	if err != nil {
		t.Fatalf("io.Reader.Read failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("io.Reader.Read: expected %d bytes, read %d", len(data), n)
	}
	if !bytes.Equal(buffer[:n], data) {
		t.Errorf("io.Reader.Read: data mismatch")
	}
}

func TestIOCopy(t *testing.T) {
	source := bytes.NewReader([]byte("Testing io.Copy"))
	rb := New(256)

	// Copy from source to ring buffer
	n, err := io.Copy(rb, source)
	if err != nil {
		t.Fatalf("io.Copy to RingBuffer failed: %v", err)
	}
	expected := int64(15) // "Testing io.Copy"
	if n != expected {
		t.Errorf("io.Copy: expected %d bytes, copied %d", expected, n)
	}

	// Verify data in ring buffer
	if rb.AvailableRead() != uint64(expected) {
		t.Errorf("After copy: expected %d bytes in buffer, got %d", expected, rb.AvailableRead())
	}

	// Read back from ring buffer manually
	buffer := make([]byte, 20)
	readN, err := rb.Read(buffer)
	if err != nil {
		t.Fatalf("Read from RingBuffer failed: %v", err)
	}

	if string(buffer[:readN]) != "Testing io.Copy" {
		t.Errorf("Data mismatch: expected %q, got %q", "Testing io.Copy", buffer[:readN])
	}
}

func TestIOReadFull(t *testing.T) {
	rb := New(256)

	// Write some data
	testData := []byte("Hello, io.ReadFull!")
	rb.Write(testData)

	// Use io.ReadFull to read exact amount
	buffer := make([]byte, len(testData))
	n, err := io.ReadFull(rb, buffer)
	if err != nil {
		t.Fatalf("io.ReadFull failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("io.ReadFull: expected %d bytes, read %d", len(testData), n)
	}
	if !bytes.Equal(buffer, testData) {
		t.Errorf("io.ReadFull: data mismatch")
	}

	// Try to read more than available
	// io.ReadFull will get partial data, then hit our ErrInsufficientData
	rb.Write([]byte("short"))
	buffer = make([]byte, 100)
	n, err = io.ReadFull(rb, buffer)
	if err == nil {
		t.Error("io.ReadFull with insufficient data: expected an error")
	}
	// Should have read the 5 bytes that were available
	if n != 5 {
		t.Errorf("io.ReadFull: expected to read 5 bytes before error, got %d", n)
	}
}

func TestIOReadAtLeast(t *testing.T) {
	rb := New(256)

	// Write data
	rb.Write([]byte("Testing ReadAtLeast"))

	// Read at least 7 bytes
	buffer := make([]byte, 20)
	n, err := io.ReadAtLeast(rb, buffer, 7)
	if err != nil {
		t.Fatalf("io.ReadAtLeast failed: %v", err)
	}
	if n < 7 {
		t.Errorf("io.ReadAtLeast: expected at least 7 bytes, read %d", n)
	}

	// Try to read at least more than buffer size
	rb.Write([]byte("more data"))
	_, err = io.ReadAtLeast(rb, buffer, 100)
	if err != io.ErrShortBuffer {
		t.Errorf("io.ReadAtLeast with impossible request: expected io.ErrShortBuffer, got %v", err)
	}
}

func TestWriteString(t *testing.T) {
	rb := New(256)

	// Use io.WriteString (which will use Write method)
	str := "Testing io.WriteString"
	n, err := io.WriteString(rb, str)
	if err != nil {
		t.Fatalf("io.WriteString failed: %v", err)
	}
	if n != len(str) {
		t.Errorf("io.WriteString: expected %d bytes, wrote %d", len(str), n)
	}

	// Read back
	buffer := make([]byte, 50)
	n, _ = rb.Read(buffer)
	if string(buffer[:n]) != str {
		t.Errorf("WriteString: expected %q, got %q", str, buffer[:n])
	}
}

func TestMultiWriter(t *testing.T) {
	rb1 := New(256)
	rb2 := New(256)

	// Create a MultiWriter that writes to both ring buffers
	multi := io.MultiWriter(rb1, rb2)

	data := []byte("Broadcast to multiple buffers")
	n, err := multi.Write(data)
	if err != nil {
		t.Fatalf("io.MultiWriter.Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("io.MultiWriter: expected %d bytes, wrote %d", len(data), n)
	}

	// Verify both buffers got the data
	buf1 := make([]byte, 50)
	n1, _ := rb1.Read(buf1)

	buf2 := make([]byte, 50)
	n2, _ := rb2.Read(buf2)

	if !bytes.Equal(buf1[:n1], data) || !bytes.Equal(buf2[:n2], data) {
		t.Error("io.MultiWriter: data mismatch in one or both buffers")
	}
}

func TestTeeReader(t *testing.T) {
	rb := New(256)
	source := bytes.NewReader([]byte("Testing io.TeeReader"))

	// Create a TeeReader that reads from source and writes to ring buffer
	tee := io.TeeReader(source, rb)

	// Read from tee (which also writes to ring buffer)
	buffer := make([]byte, 50)
	n, err := tee.Read(buffer)
	if err != nil && err != io.EOF {
		t.Fatalf("TeeReader.Read failed: %v", err)
	}

	// Verify ring buffer also got the data
	rbBuffer := make([]byte, 50)
	rbN, _ := rb.Read(rbBuffer)

	if !bytes.Equal(buffer[:n], rbBuffer[:rbN]) {
		t.Error("io.TeeReader: data mismatch between direct read and ring buffer")
	}
}

func ExampleRingBuffer_ioWriter() {
	rb := New(256)

	// Use as io.Writer
	var w io.Writer = rb
	io.WriteString(w, "Hello, ")
	io.WriteString(w, "World!")

	// Read back
	buffer := make([]byte, 20)
	n, _ := rb.Read(buffer)
	fmt.Printf("%s\n", buffer[:n])
	// Output:
	// Hello, World!
}

func ExampleRingBuffer_ioReader() {
	rb := New(256)

	// Write some data
	rb.Write([]byte("Data for io.Reader"))

	// Use as io.Reader
	var r io.Reader = rb
	buffer := make([]byte, 50)
	n, _ := r.Read(buffer)

	fmt.Printf("Read %d bytes: %s\n", n, buffer[:n])
	// Output:
	// Read 18 bytes: Data for io.Reader
}

func ExampleRingBuffer_ioCopy() {
	rb := New(256)

	// Copy from a source to the ring buffer
	source := strings.NewReader("Copied via io.Copy")
	io.Copy(rb, source)

	// Copy from ring buffer to destination
	var dest bytes.Buffer
	io.Copy(&dest, rb)

	fmt.Println(dest.String())
	// Output:
	// Copied via io.Copy
}
