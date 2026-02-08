package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"musictools/pkg/ringbuffer"
)

const (
	sampleRate     = 44100
	channels       = 2
	bytesPerSample = 2
	bufferSize     = 8192
)

func main() {
	fmt.Println("Zero-Copy Audio Processing Demo")
	fmt.Println("================================")

	// Create ring buffer for audio samples
	rb := ringbuffer.New(bufferSize)

	var wg sync.WaitGroup
	wg.Add(2)

	// Statistics
	var totalProcessed uint64
	var copyCount int

	// Producer: simulates audio input device
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			// Generate audio samples (stereo, 16-bit)
			samples := generateSamples(256, float64(i)*0.1)

			// Wait for space
			for rb.AvailableWrite() < uint64(len(samples)) {
				time.Sleep(time.Microsecond * 10)
			}

			rb.Write(samples)
			time.Sleep(time.Millisecond * 2)
		}
		fmt.Println("Producer: finished generating audio")
	}()

	// Consumer: zero-copy audio processing
	go func() {
		defer wg.Done()

		for {
			// Check if we have at least one frame of audio
			frameSize := uint64(channels * bytesPerSample)
			if rb.AvailableRead() < frameSize {
				if totalProcessed >= 100*256 {
					break
				}
				time.Sleep(time.Microsecond * 10)
				continue
			}

			// Zero-copy access to audio data
			first, second, total := rb.ReadSlices()

			// Process first slice without copying
			processedFirst := processAudioSlice(first)
			totalProcessed += uint64(len(first))

			// Process second slice if data wrapped
			var processedSecond int
			if second != nil {
				processedSecond = processAudioSlice(second)
				totalProcessed += uint64(len(second))
				copyCount++
			}

			// Report every 10th chunk
			if copyCount%10 == 0 {
				fmt.Printf("Processed: %d bytes total, first=%d, second=%d (zero copies!)\n",
					total, processedFirst, processedSecond)
			}

			// Consume the processed data
			rb.Consume(total)
			copyCount++
		}

		fmt.Printf("\nConsumer: finished processing %d bytes in %d chunks\n",
			totalProcessed, copyCount)
	}()

	wg.Wait()
	fmt.Println("\nDemo completed!")
}

// generateSamples creates stereo audio samples (sine wave)
func generateSamples(numBytes int, phase float64) []byte {
	numSamples := numBytes / bytesPerSample
	samples := make([]byte, numBytes)

	for i := 0; i < numSamples/channels; i++ {
		// Generate sine wave sample
		t := float64(i) / float64(sampleRate)
		amplitude := 0.3 * math.Sin(2*math.Pi*440*t+phase) // 440 Hz sine
		sample := int16(amplitude * 32767)

		// Stereo (same sample for both channels)
		binary.LittleEndian.PutUint16(samples[i*4:], uint16(sample))   // Left
		binary.LittleEndian.PutUint16(samples[i*4+2:], uint16(sample)) // Right
	}

	return samples
}

// processAudioSlice simulates audio processing (e.g., applying effects, analyzing)
// This function receives a slice directly from the ring buffer - zero copy!
func processAudioSlice(audioData []byte) int {
	// In a real application, you would:
	// - Apply audio effects (reverb, EQ, compression)
	// - Analyze audio (FFT, peak detection, etc.)
	// - Send to audio output device
	// - All without copying the data!

	// For demonstration, just count samples
	numSamples := len(audioData) / bytesPerSample

	// Simulate some processing time
	var sum int64
	for i := 0; i < len(audioData); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(audioData[i:]))
		sum += int64(sample)
	}

	return numSamples
}
