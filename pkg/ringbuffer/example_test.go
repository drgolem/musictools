package ringbuffer_test

import (
	"fmt"
	"musictools/pkg/ringbuffer"
	"sync"
	"time"
)

func Example() {
	// Create a 1KB ring buffer
	rb := ringbuffer.New(1024)

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer goroutine
	go func() {
		defer wg.Done()
		data := []byte("Hello from producer!")

		for rb.AvailableWrite() < uint64(len(data)) {
			time.Sleep(time.Microsecond)
		}

		n, err := rb.Write(data)
		if err != nil {
			fmt.Printf("Write error: %v\n", err)
			return
		}
		fmt.Printf("Wrote %d bytes\n", n)
	}()

	// Consumer goroutine
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond) // Wait for producer

		buffer := make([]byte, 100)
		n, err := rb.Read(buffer)
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
			return
		}
		fmt.Printf("Read %d bytes: %s\n", n, buffer[:n])
	}()

	wg.Wait()
	// Output:
	// Wrote 20 bytes
	// Read 20 bytes: Hello from producer!
}

func ExampleNew() {
	// Create a ring buffer with 512 bytes capacity
	// The size will be rounded to the nearest power of 2
	rb := ringbuffer.New(512)

	fmt.Printf("Buffer size: %d bytes\n", rb.Size())
	fmt.Printf("Available for writing: %d bytes\n", rb.AvailableWrite())
	// Output:
	// Buffer size: 512 bytes
	// Available for writing: 512 bytes
}

func ExampleRingBuffer_Write() {
	rb := ringbuffer.New(256)

	data := []byte("Hello, World!")
	n, err := rb.Write(data)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Wrote %d bytes\n", n)
	fmt.Printf("Available to read: %d bytes\n", rb.AvailableRead())
	// Output:
	// Wrote 13 bytes
	// Available to read: 13 bytes
}

func ExampleRingBuffer_Read() {
	rb := ringbuffer.New(256)

	// Write some data first
	rb.Write([]byte("Hello!"))

	// Read it back
	buffer := make([]byte, 10)
	n, err := rb.Read(buffer)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Read %d bytes: %s\n", n, buffer[:n])
	// Output:
	// Read 6 bytes: Hello!
}

func ExampleRingBuffer_ReadSlices() {
	rb := ringbuffer.New(256)

	// Write some data
	rb.Write([]byte("Zero-copy reading!"))

	// Get zero-copy slices
	first, second, total := rb.ReadSlices()

	fmt.Printf("Total available: %d bytes\n", total)
	fmt.Printf("First slice: %s\n", first)
	if second != nil {
		fmt.Printf("Second slice: %s\n", second)
	} else {
		fmt.Println("Second slice: (none - data is contiguous)")
	}

	// Process the data directly without copying...
	// When done, consume it
	rb.Consume(total)

	fmt.Printf("Remaining after consume: %d bytes\n", rb.AvailableRead())
	// Output:
	// Total available: 18 bytes
	// First slice: Zero-copy reading!
	// Second slice: (none - data is contiguous)
	// Remaining after consume: 0 bytes
}

func ExampleRingBuffer_PeekContiguous() {
	rb := ringbuffer.New(256)

	// Write audio data
	rb.Write([]byte("Audio sample data"))

	// Peek at the data without consuming it
	data := rb.PeekContiguous()
	fmt.Printf("Peeked %d bytes: %s\n", len(data), data)

	// Data is still available
	fmt.Printf("Still available: %d bytes\n", rb.AvailableRead())

	// Process and consume part of it
	processSize := uint64(5)
	rb.Consume(processSize)

	fmt.Printf("After consuming %d bytes: %d bytes remaining\n", processSize, rb.AvailableRead())
	// Output:
	// Peeked 17 bytes: Audio sample data
	// Still available: 17 bytes
	// After consuming 5 bytes: 12 bytes remaining
}

func ExampleRingBuffer_ReadSlices_wrapped() {
	// Small buffer to demonstrate wrap-around
	rb := ringbuffer.New(16)

	// Fill and drain to position the pointers
	rb.Write([]byte("1234567"))
	temp := make([]byte, 7)
	rb.Read(temp)

	// Write data that wraps around
	rb.Write([]byte("abcdefghijk"))

	// ReadSlices returns two slices when data wraps
	first, second, total := rb.ReadSlices()

	fmt.Printf("Total: %d bytes\n", total)
	fmt.Printf("First: %s\n", first)
	fmt.Printf("Second: %s\n", second)

	// Combine and process both slices
	combined := append(append([]byte{}, first...), second...)
	fmt.Printf("Combined: %s\n", combined)

	// Consume all
	rb.Consume(total)
	// Output:
	// Total: 11 bytes
	// First: abcdefghi
	// Second: jk
	// Combined: abcdefghijk
}

func Example_zeroCopyAudioProcessing() {
	// Realistic audio processing example
	rb := ringbuffer.New(4096)

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer: audio capture thread
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			// Simulate audio capture
			audioData := []byte(fmt.Sprintf("Audio chunk %d\n", i))

			for rb.AvailableWrite() < uint64(len(audioData)) {
				time.Sleep(time.Microsecond * 100)
			}

			rb.Write(audioData)
			time.Sleep(time.Millisecond * 5)
		}
	}()

	// Consumer: zero-copy audio processing
	go func() {
		defer wg.Done()
		processed := 0

		for processed < 3 {
			if rb.AvailableRead() == 0 {
				time.Sleep(time.Microsecond * 100)
				continue
			}

			// Zero-copy access to audio data
			first, second, total := rb.ReadSlices()

			// Process first slice directly (no copy!)
			fmt.Printf("Processing %d bytes from first slice\n", len(first))

			// Process second slice if it exists (wrapped data)
			if second != nil {
				fmt.Printf("Processing %d bytes from second slice\n", len(second))
			}

			// After processing, consume the data
			rb.Consume(total)
			processed++
		}
	}()

	wg.Wait()
	// Output:
	// Processing 14 bytes from first slice
	// Processing 14 bytes from first slice
	// Processing 14 bytes from first slice
}
