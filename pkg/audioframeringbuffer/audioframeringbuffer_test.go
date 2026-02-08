package audioframeringbuffer

import (
	"sync"
	"testing"

	"musictools/pkg/audioframe"
)

func TestNewRoundsToPowerOf2(t *testing.T) {
	tests := []struct {
		input    uint64
		expected uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{100, 128},
		{1000, 1024},
		{1024, 1024},
	}

	for _, tt := range tests {
		rb := New(tt.input)
		if rb.Size() != tt.expected {
			t.Errorf("New(%d): got size %d, want %d", tt.input, rb.Size(), tt.expected)
		}
	}
}

func TestWriteRead(t *testing.T) {
	rb := New(16)

	// Create test frames
	frames := []audioframe.AudioFrame{
		{
			Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
			SamplesCount: 1024,
			Audio:        []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			Format:       audioframe.FrameFormat{SampleRate: 48000, Channels: 1, BitsPerSample: 24},
			SamplesCount: 512,
			Audio:        []byte{0x05, 0x06, 0x07, 0x08},
		},
		{
			Format:       audioframe.FrameFormat{SampleRate: 96000, Channels: 6, BitsPerSample: 32},
			SamplesCount: 2048,
			Audio:        []byte{0x09, 0x0A, 0x0B, 0x0C},
		},
	}

	// Write frames
	written, err := rb.Write(frames)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if written != len(frames) {
		t.Fatalf("Write: got %d frames, want %d", written, len(frames))
	}

	// Check available
	if rb.AvailableRead() != 3 {
		t.Errorf("AvailableRead: got %d, want 3", rb.AvailableRead())
	}
	if rb.AvailableWrite() != 13 {
		t.Errorf("AvailableWrite: got %d, want 13", rb.AvailableWrite())
	}

	// Read frames
	readFrames, err := rb.Read(3)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify frames
	if len(readFrames) != 3 {
		t.Fatalf("Read returned %d frames, want 3", len(readFrames))
	}

	for i := 0; i < 3; i++ {
		if readFrames[i].Format.SampleRate != frames[i].Format.SampleRate {
			t.Errorf("Frame %d: SampleRate mismatch", i)
		}
		if readFrames[i].SamplesCount != frames[i].SamplesCount {
			t.Errorf("Frame %d: SamplesCount mismatch", i)
		}
		if len(readFrames[i].Audio) != len(frames[i].Audio) {
			t.Errorf("Frame %d: Audio length mismatch", i)
		}
	}
}

func TestReadPartial(t *testing.T) {
	rb := New(16)

	// Write 5 frames
	frames := make([]audioframe.AudioFrame, 5)
	for i := range frames {
		frames[i] = audioframe.AudioFrame{
			Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
			SamplesCount: uint16(i + 1),
			Audio:        []byte{byte(i)},
		}
	}

	_, err := rb.Write(frames)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read only 3 frames
	readFrames, err := rb.Read(3)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(readFrames) != 3 {
		t.Errorf("Read returned %d frames, want 3", len(readFrames))
	}

	// Verify we got the first 3 frames
	for i := 0; i < 3; i++ {
		if readFrames[i].SamplesCount != uint16(i+1) {
			t.Errorf("Frame %d: got SamplesCount %d, want %d", i, readFrames[i].SamplesCount, i+1)
		}
	}

	// Check remaining
	if rb.AvailableRead() != 2 {
		t.Errorf("AvailableRead: got %d, want 2", rb.AvailableRead())
	}

	// Read remaining
	readFrames, err = rb.Read(10) // Request more than available
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(readFrames) != 2 {
		t.Errorf("Read returned %d frames, want 2", len(readFrames))
	}
}

func TestWriteInsufficientSpace(t *testing.T) {
	rb := New(4) // Capacity is 4

	frames := make([]audioframe.AudioFrame, 5) // Try to write 5
	written, err := rb.Write(frames)
	if written != 4 {
		t.Errorf("Expected to write 4 frames, got %d", written)
	}
	if err != nil {
		t.Errorf("Expected nil error for partial write, got %v", err)
	}

	// Try to write when completely full
	_, err = rb.Write([]audioframe.AudioFrame{{Format: audioframe.FrameFormat{}}})
	if err != ErrInsufficientSpace {
		t.Errorf("Expected ErrInsufficientSpace when full, got %v", err)
	}
}

func TestReadEmptyBuffer(t *testing.T) {
	rb := New(16)

	_, err := rb.Read(1)
	if err != ErrInsufficientData {
		t.Errorf("Expected ErrInsufficientData, got %v", err)
	}
}

func TestWrapAround(t *testing.T) {
	rb := New(4) // Small buffer to force wrap-around

	// Write 3 frames
	frames1 := make([]audioframe.AudioFrame, 3)
	for i := range frames1 {
		frames1[i] = audioframe.AudioFrame{
			Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
			SamplesCount: uint16(i + 1),
			Audio:        []byte{byte(i)},
		}
	}

	written, err := rb.Write(frames1)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if written != len(frames1) {
		t.Fatalf("Write: got %d frames, want %d", written, len(frames1))
	}

	// Read 2 frames (leaves 1 in buffer)
	_, err = rb.Read(2)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Write 3 more frames (will wrap around)
	frames2 := make([]audioframe.AudioFrame, 3)
	for i := range frames2 {
		frames2[i] = audioframe.AudioFrame{
			Format:       audioframe.FrameFormat{SampleRate: 48000, Channels: 1, BitsPerSample: 24},
			SamplesCount: uint16(i + 10),
			Audio:        []byte{byte(i + 10)},
		}
	}

	written, err = rb.Write(frames2)
	if err != nil {
		t.Fatalf("Write after wrap failed: %v", err)
	}
	if written != len(frames2) {
		t.Fatalf("Write after wrap: got %d frames, want %d", written, len(frames2))
	}

	// Should have 4 frames total (1 old + 3 new)
	if rb.AvailableRead() != 4 {
		t.Errorf("AvailableRead: got %d, want 4", rb.AvailableRead())
	}

	// Read all frames
	readFrames, err := rb.Read(4)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify we got the remaining old frame + 3 new frames
	if len(readFrames) != 4 {
		t.Errorf("Read returned %d frames, want 4", len(readFrames))
	}

	// First frame should be the last of frames1
	if readFrames[0].SamplesCount != 3 {
		t.Errorf("First frame: got SamplesCount %d, want 3", readFrames[0].SamplesCount)
	}

	// Next frames should be frames2
	for i := 1; i < 4; i++ {
		if readFrames[i].SamplesCount != uint16(i-1+10) {
			t.Errorf("Frame %d: got SamplesCount %d, want %d", i, readFrames[i].SamplesCount, i-1+10)
		}
	}
}

func TestReset(t *testing.T) {
	rb := New(16)

	// Write some frames
	frames := make([]audioframe.AudioFrame, 3)
	_, err := rb.Write(frames)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Reset
	rb.Reset()

	// Check empty
	if rb.AvailableRead() != 0 {
		t.Errorf("After reset: AvailableRead got %d, want 0", rb.AvailableRead())
	}
	if rb.AvailableWrite() != rb.Size() {
		t.Errorf("After reset: AvailableWrite got %d, want %d", rb.AvailableWrite(), rb.Size())
	}
}

func TestEmptyWriteRead(t *testing.T) {
	rb := New(16)

	// Write empty slice
	written, err := rb.Write([]audioframe.AudioFrame{})
	if err != nil {
		t.Errorf("Write empty slice failed: %v", err)
	}
	if written != 0 {
		t.Errorf("Write empty: got %d, want 0", written)
	}

	// Read zero frames
	frames, err := rb.Read(0)
	if err != nil {
		t.Errorf("Read(0) failed: %v", err)
	}
	if frames != nil {
		t.Errorf("Read(0) returned non-nil: %v", frames)
	}

	// Read negative (should return nil)
	frames, err = rb.Read(-1)
	if err != nil {
		t.Errorf("Read(-1) failed: %v", err)
	}
	if frames != nil {
		t.Errorf("Read(-1) returned non-nil: %v", frames)
	}
}

func TestDeepCopyAudioBuffer(t *testing.T) {
	rb := New(16)

	// Create frame with reusable buffer
	audioBuffer := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	frame := audioframe.AudioFrame{
		Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
		SamplesCount: 1024,
		Audio:        audioBuffer,
	}

	// Write frame to buffer
	written, err := rb.Write([]audioframe.AudioFrame{frame})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if written != 1 {
		t.Fatalf("Write: got %d frames, want 1", written)
	}

	// Modify the original buffer (simulating buffer reuse)
	audioBuffer[0] = 0xFF
	audioBuffer[1] = 0xFF
	audioBuffer[2] = 0xFF
	audioBuffer[3] = 0xFF

	// Read back and verify data is NOT corrupted
	readFrames, err := rb.Read(1)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(readFrames) != 1 {
		t.Fatalf("Read returned %d frames, want 1", len(readFrames))
	}

	// Verify the data in ringbuffer was not affected by buffer reuse
	if readFrames[0].Audio[0] != 0xAA {
		t.Errorf("Audio[0]: got 0x%02X, want 0xAA", readFrames[0].Audio[0])
	}
	if readFrames[0].Audio[1] != 0xBB {
		t.Errorf("Audio[1]: got 0x%02X, want 0xBB", readFrames[0].Audio[1])
	}
	if readFrames[0].Audio[2] != 0xCC {
		t.Errorf("Audio[2]: got 0x%02X, want 0xCC", readFrames[0].Audio[2])
	}
	if readFrames[0].Audio[3] != 0xDD {
		t.Errorf("Audio[3]: got 0x%02X, want 0xDD", readFrames[0].Audio[3])
	}
}

func TestConcurrentProducerConsumer(t *testing.T) {
	rb := New(256)

	const numFrames = 10000
	const batchSize = 10

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numFrames; i += batchSize {
			frames := make([]audioframe.AudioFrame, batchSize)
			for j := 0; j < batchSize; j++ {
				frames[j] = audioframe.AudioFrame{
					Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
					SamplesCount: uint16(i + j),
					Audio:        []byte{byte(i + j)},
				}
			}

			// Retry until all frames written
			toWrite := frames
			for len(toWrite) > 0 {
				written, _ := rb.Write(toWrite)
				toWrite = toWrite[written:]
				// Yield to consumer if partial write
			}
		}
	}()

	// Consumer goroutine
	received := 0
	go func() {
		defer wg.Done()
		for received < numFrames {
			frames, err := rb.Read(batchSize)
			if err == ErrInsufficientData {
				// Yield to producer
				continue
			}
			if err != nil {
				t.Errorf("Consumer read error: %v", err)
				return
			}

			// Verify frames
			for _, frame := range frames {
				if frame.SamplesCount != uint16(received) {
					t.Errorf("Frame %d: got SamplesCount %d, want %d", received, frame.SamplesCount, received)
				}
				received++
			}
		}
	}()

	wg.Wait()

	if received != numFrames {
		t.Errorf("Received %d frames, want %d", received, numFrames)
	}
}

func BenchmarkWrite(b *testing.B) {
	rb := New(8192)

	frames := make([]audioframe.AudioFrame, 10)
	for i := range frames {
		frames[i] = audioframe.AudioFrame{
			Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
			SamplesCount: 1024,
			Audio:        make([]byte, 4096),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(frames)
		rb.Reset() // Reset to avoid filling up
	}
}

func BenchmarkRead(b *testing.B) {
	rb := New(8192)

	// Pre-fill buffer
	frames := make([]audioframe.AudioFrame, 1000)
	for i := range frames {
		frames[i] = audioframe.AudioFrame{
			Format:       audioframe.FrameFormat{SampleRate: 44100, Channels: 2, BitsPerSample: 16},
			SamplesCount: 1024,
			Audio:        make([]byte, 4096),
		}
	}
	rb.Write(frames)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Read(10)
		if rb.AvailableRead() < 10 {
			rb.Reset()
			rb.Write(frames)
		}
	}
}
