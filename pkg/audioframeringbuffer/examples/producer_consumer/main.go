package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/drgolem/musictools/pkg/audioframe"
	"github.com/drgolem/musictools/pkg/audioframeringbuffer"
)

func main() {
	fmt.Println("AudioFrame RingBuffer - Producer/Consumer Example")
	fmt.Println("================================================")
	fmt.Println()

	// Create ring buffer for 256 frames
	rb := audioframeringbuffer.New(256)
	fmt.Printf("Ring buffer created: capacity=%d frames\n\n", rb.Size())

	const totalFrames = 1000
	const batchSize = 10

	var wg sync.WaitGroup
	wg.Add(2)

	// Statistics
	producedCount := 0
	consumedCount := 0

	// Producer goroutine
	go func() {
		defer wg.Done()
		fmt.Println("[Producer] Starting...")

		for i := 0; i < totalFrames; i += batchSize {
			// Create batch of frames
			batch := make([]audioframe.AudioFrame, batchSize)
			for j := 0; j < batchSize; j++ {
				frameNum := i + j
				batch[j] = audioframe.AudioFrame{
					Format: audioframe.FrameFormat{
						SampleRate:    44100,
						Channels:      2,
						BitsPerSample: 16,
					},
					SamplesCount: 1024,
					Audio:        make([]byte, 4096), // 1024 samples × 2 channels × 2 bytes
				}
				// Put frame number in first byte for verification
				batch[j].Audio[0] = byte(frameNum % 256)
			}

			// Write to ring buffer (handles partial writes)
			toWrite := batch
			for len(toWrite) > 0 {
				written, _ := rb.Write(toWrite)
				if written > 0 {
					producedCount += written
					toWrite = toWrite[written:]
					if producedCount%100 == 0 {
						fmt.Printf("[Producer] Produced %d frames (available: %d)\n",
							producedCount, rb.AvailableRead())
					}
				} else {
					// Buffer full, yield to consumer
					time.Sleep(time.Microsecond)
				}
			}

			// Simulate encoding time
			time.Sleep(100 * time.Microsecond)
		}

		fmt.Printf("[Producer] Finished! Total produced: %d frames\n", producedCount)
	}()

	// Consumer goroutine
	go func() {
		defer wg.Done()
		fmt.Println("[Consumer] Starting...")
		time.Sleep(10 * time.Millisecond) // Let producer get ahead

		for consumedCount < totalFrames {
			frames, err := rb.Read(batchSize)
			if err == audioframeringbuffer.ErrInsufficientData {
				// Buffer empty, yield to producer
				time.Sleep(time.Microsecond)
				continue
			}

			// Process frames
			for i, frame := range frames {
				// Verify frame data
				expectedFrameNum := (consumedCount + i) % 256
				actualFrameNum := int(frame.Audio[0])
				if actualFrameNum != expectedFrameNum {
					fmt.Printf("[Consumer] ERROR: Frame %d mismatch! Expected %d, got %d\n",
						consumedCount+i, expectedFrameNum, actualFrameNum)
				}

				// Verify format
				if frame.Format.SampleRate != 44100 ||
					frame.Format.Channels != 2 ||
					frame.Format.BitsPerSample != 16 {
					fmt.Printf("[Consumer] ERROR: Invalid format in frame %d\n", consumedCount+i)
				}
			}

			consumedCount += len(frames)
			if consumedCount%100 == 0 {
				fmt.Printf("[Consumer] Consumed %d frames (available: %d)\n",
					consumedCount, rb.AvailableRead())
			}

			// Simulate processing time
			time.Sleep(150 * time.Microsecond)
		}

		fmt.Printf("[Consumer] Finished! Total consumed: %d frames\n", consumedCount)
	}()

	// Wait for completion
	wg.Wait()

	fmt.Println()
	fmt.Println("Results:")
	fmt.Printf("  Produced: %d frames\n", producedCount)
	fmt.Printf("  Consumed: %d frames\n", consumedCount)
	fmt.Printf("  Remaining in buffer: %d frames\n", rb.AvailableRead())

	if producedCount == totalFrames && consumedCount == totalFrames {
		fmt.Println()
		fmt.Println("✓ SUCCESS: All frames produced and consumed correctly!")
	} else {
		fmt.Println()
		fmt.Println("✗ ERROR: Frame count mismatch!")
	}
}
