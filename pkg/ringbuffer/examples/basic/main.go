package main

import (
	"fmt"
	"sync"
	"time"

	"learnRingbuffer/pkg/ringbuffer"
)

func main() {
	// Create a 1KB ring buffer
	rb := ringbuffer.New(1024)

	fmt.Println("Lock-free SPSC Ring Buffer Demo")
	fmt.Printf("Buffer size: %d bytes\n\n", rb.Size())

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer goroutine - simulates audio input
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			// Simulate audio chunk (e.g., 64 bytes)
			audioData := make([]byte, 64)
			for j := range audioData {
				audioData[j] = byte(i*10 + j)
			}

			// Wait for space if buffer is full
			for rb.AvailableWrite() < uint64(len(audioData)) {
				time.Sleep(time.Millisecond)
			}

			n, err := rb.Write(audioData)
			if err != nil {
				fmt.Printf("Producer error: %v\n", err)
				return
			}

			fmt.Printf("Producer: wrote %d bytes (chunk %d), available: %d bytes\n",
				n, i, rb.AvailableRead())

			time.Sleep(10 * time.Millisecond)
		}
		fmt.Println("Producer: finished")
	}()

	// Consumer goroutine - simulates audio output
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // Start slightly after producer

		totalRead := 0
		for totalRead < 640 { // 10 chunks * 64 bytes
			readBuf := make([]byte, 64)

			// Wait for data if buffer is empty
			for rb.AvailableRead() == 0 {
				time.Sleep(time.Millisecond)
			}

			n, err := rb.Read(readBuf)
			if err != nil {
				fmt.Printf("Consumer error: %v\n", err)
				return
			}

			totalRead += n
			fmt.Printf("Consumer: read %d bytes, total: %d, remaining: %d bytes\n",
				n, totalRead, rb.AvailableRead())

			time.Sleep(15 * time.Millisecond)
		}
		fmt.Println("Consumer: finished")
	}()

	wg.Wait()
	fmt.Println("\nDemo completed successfully!")
}
