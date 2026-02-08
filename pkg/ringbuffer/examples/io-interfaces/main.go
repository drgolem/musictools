package main

import (
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"musictools/pkg/ringbuffer"
)

func main() {
	fmt.Println("io.Reader/io.Writer Interface Demo")
	fmt.Println("===================================")

	// Demo 1: Using io.Copy for data transfer
	demo1_ioCopy()

	// Demo 2: Compression pipeline using io.Writer
	demo2_compressionPipeline()

	// Demo 3: MultiWriter broadcasting
	demo3_multiWriter()
}

func demo1_ioCopy() {
	fmt.Println("Demo 1: Using io.Copy")
	fmt.Println("---------------------")

	rb := ringbuffer.New(1024)

	// Generate random data
	source := make([]byte, 256)
	rand.Read(source)

	// Copy to ring buffer using io.Copy
	written, err := io.Copy(rb, io.LimitReader(rand.Reader, 256))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Copied %d bytes to ring buffer using io.Copy\n", written)
	fmt.Printf("Ring buffer now has %d bytes available\n\n", rb.AvailableRead())
}

func demo2_compressionPipeline() {
	fmt.Println("Demo 2: Compression Pipeline")
	fmt.Println("----------------------------")

	// Create two ring buffers for a compression pipeline
	inputBuffer := ringbuffer.New(4096)
	outputBuffer := ringbuffer.New(4096)

	var wg sync.WaitGroup
	wg.Add(3)

	// Producer: Generate data
	go func() {
		defer wg.Done()
		data := []byte("This is test data that will be compressed. " +
			"The ring buffer implements io.Writer, so it works seamlessly " +
			"with the compression pipeline! ")

		// Write data multiple times to have something to compress
		for i := 0; i < 5; i++ {
			// Use io.WriteString (which uses io.Writer interface)
			n, err := io.WriteString(inputBuffer, string(data))
			if err != nil {
				fmt.Printf("Write error: %v\n", err)
				return
			}
			fmt.Printf("Producer: wrote %d bytes\n", n)
			time.Sleep(time.Millisecond * 10)
		}
	}()

	// Compressor: Read from input, compress, write to output
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond * 20) // Let some data accumulate

		// Create a gzip writer that writes to the output buffer
		// This works because outputBuffer implements io.Writer!
		gzWriter := gzip.NewWriter(outputBuffer)

		// Copy data from input buffer through gzip to output buffer
		// Both buffers implement io.Reader and io.Writer
		buffer := make([]byte, 128)
		for i := 0; i < 5; i++ {
			// Wait for data
			for inputBuffer.AvailableRead() == 0 {
				time.Sleep(time.Millisecond)
			}

			n, _ := inputBuffer.Read(buffer)
			if n > 0 {
				gzWriter.Write(buffer[:n])
				fmt.Printf("Compressor: compressed %d bytes\n", n)
			}
		}

		gzWriter.Close()
		fmt.Printf("Compression complete. Compressed size: %d bytes\n",
			outputBuffer.AvailableRead())
	}()

	// Consumer: Read compressed data
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond * 100) // Wait for compression

		// Read compressed data
		compressed := make([]byte, outputBuffer.AvailableRead())
		n, _ := outputBuffer.Read(compressed)

		fmt.Printf("Consumer: read %d bytes of compressed data\n", n)
	}()

	wg.Wait()
	fmt.Println()
}

func demo3_multiWriter() {
	fmt.Println("Demo 3: MultiWriter Broadcasting")
	fmt.Println("--------------------------------")

	// Create multiple ring buffers
	buffer1 := ringbuffer.New(1024)
	buffer2 := ringbuffer.New(1024)
	buffer3 := ringbuffer.New(1024)

	// Create a MultiWriter that broadcasts to all buffers
	// This works because all buffers implement io.Writer!
	multi := io.MultiWriter(buffer1, buffer2, buffer3)

	// Write once, broadcast to all
	message := "Broadcasting to multiple ring buffers!"
	n, err := io.WriteString(multi, message)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Wrote %d bytes to MultiWriter\n", n)

	// Verify all buffers received the data
	fmt.Printf("Buffer 1: %d bytes available\n", buffer1.AvailableRead())
	fmt.Printf("Buffer 2: %d bytes available\n", buffer2.AvailableRead())
	fmt.Printf("Buffer 3: %d bytes available\n", buffer3.AvailableRead())

	// Read from one buffer to verify
	readBuf := make([]byte, 100)
	n, _ = buffer1.Read(readBuf)
	fmt.Printf("Read from buffer 1: %s\n", readBuf[:n])
}
