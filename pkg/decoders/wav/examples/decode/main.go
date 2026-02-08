package main

import (
	"fmt"
	"log"
	"os"

	"musictools/pkg/decoders/wav"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: decode <input.wav>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Decodes a WAV file and prints information about it")
		os.Exit(1)
	}

	inputFile := os.Args[1]

	// Create decoder
	decoder := wav.NewDecoder()

	// Open WAV file
	fmt.Printf("Opening: %s\n", inputFile)
	if err := decoder.Open(inputFile); err != nil {
		log.Fatalf("Failed to open WAV file: %v", err)
	}
	defer decoder.Close()

	// Get format
	rate, channels, bps := decoder.GetFormat()
	fmt.Printf("Sample Rate: %d Hz\n", rate)
	fmt.Printf("Channels: %d\n", channels)
	fmt.Printf("Bits Per Sample: %d\n", bps)
	fmt.Println()

	// Calculate buffer size for decoding
	samplesToRead := 1024
	bytesPerSample := bps / 8
	bufferSize := samplesToRead * channels * bytesPerSample
	buffer := make([]byte, bufferSize)

	// Decode and count samples
	totalSamples := 0
	iterations := 0

	fmt.Printf("Decoding %d samples at a time...\n", samplesToRead)

	for {
		samplesRead, err := decoder.DecodeSamples(samplesToRead, buffer)
		if err != nil || samplesRead == 0 {
			break
		}

		totalSamples += samplesRead
		iterations++

		if iterations <= 3 || iterations%100 == 0 {
			bytesRead := samplesRead * channels * bytesPerSample
			fmt.Printf("Iteration %d: Read %d samples (%d bytes)\n",
				iterations, samplesRead, bytesRead)
		}
	}

	fmt.Println()
	fmt.Printf("Total samples decoded: %d\n", totalSamples)
	fmt.Printf("Total iterations: %d\n", iterations)

	duration := float64(totalSamples) / float64(rate)
	fmt.Printf("Duration: %.2f seconds\n", duration)

	totalBytes := totalSamples * channels * bytesPerSample
	fmt.Printf("Total audio data: %d bytes (%.2f MB)\n",
		totalBytes, float64(totalBytes)/(1024*1024))
}
