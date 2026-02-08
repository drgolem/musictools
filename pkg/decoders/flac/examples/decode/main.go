package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"learnRingbuffer/pkg/decoders/flac"
)

// AudioMetadata contains format information for the decoded audio
type AudioMetadata struct {
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	BitsPerSample int    `json:"bits_per_sample"`
	SourceFile    string `json:"source_file"`
	RawFile       string `json:"raw_file"`
}

func main() {
	// Setup structured logging to stderr
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: decode <input.flac> [output_prefix|--pipe|-]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Decodes a FLAC file to raw PCM data and metadata")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  # Save to files (creates music.raw and music.meta)")
		fmt.Fprintln(os.Stderr, "  decode music.flac")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  # Save with custom prefix")
		fmt.Fprintln(os.Stderr, "  decode music.flac output")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  # Pipe mode: stream directly to ffplay (no files)")
		fmt.Fprintln(os.Stderr, "  decode music.flac --pipe | ffplay -f s16le -ar 44100 -ch_layout stereo -")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  # Convert to WAV on-the-fly")
		fmt.Fprintln(os.Stderr, "  decode music.flac - | ffmpeg -f s16le -ar 44100 -ch_layout stereo -i - output.wav")
		os.Exit(1)
	}

	inputFile := os.Args[1]

	// Check if piping to stdout
	pipeMode := false
	if len(os.Args) >= 3 && (os.Args[2] == "--pipe" || os.Args[2] == "-") {
		pipeMode = true
	}

	if pipeMode {
		decodeToStdout(inputFile)
		return
	}

	// Determine output prefix
	outputPrefix := "output"
	if len(os.Args) >= 3 {
		outputPrefix = os.Args[2]
	} else {
		// Use input filename without extension as prefix
		base := filepath.Base(inputFile)
		outputPrefix = strings.TrimSuffix(base, filepath.Ext(base))
	}

	rawFile := outputPrefix + ".raw"
	metaFile := outputPrefix + ".meta"

	slog.Info("Starting decode",
		"input", inputFile,
		"output_raw", rawFile,
		"output_meta", metaFile)

	// Create decoder
	decoder := flac.NewDecoder()
	defer decoder.Close()

	// Open FLAC file
	err := decoder.Open(inputFile)
	if err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	// Get audio format
	rate, channels, bps := decoder.GetFormat()
	slog.Info("Audio format",
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	// Create output file for raw PCM data
	outFile, err := os.Create(rawFile)
	if err != nil {
		slog.Error("Failed to create output file", "error", err)
		os.Exit(1)
	}
	defer outFile.Close()

	// Decode and write raw PCM data
	audioSamples := 4 * 1024                                     // Number of samples to decode
	bytesPerSample := bps / 8                                    // Bytes per sample (16, 24, or 32 bits)
	audioBufferBytes := audioSamples * channels * bytesPerSample // Buffer size in bytes
	buffer := make([]byte, audioBufferBytes)

	totalBytes := 0
	chunkCount := 0

	slog.Info("Decoding started")
	for {
		// DecodeSamples expects number of SAMPLES, returns number of SAMPLES
		samplesRead, err := decoder.DecodeSamples(audioSamples, buffer)

		// Check for errors first
		if err != nil {
			break
		}

		// Check if no data was read (EOF)
		if samplesRead == 0 {
			break
		}

		// Calculate bytes to write: samples * channels * bytes_per_sample
		bytesToWrite := samplesRead * channels * bytesPerSample

		// Write the decoded data
		written, err := outFile.Write(buffer[:bytesToWrite])
		if err != nil {
			slog.Error("Failed to write output", "error", err)
			os.Exit(1)
		}
		totalBytes += written
		chunkCount++

		// Log progress every 100 chunks
		if chunkCount%100 == 0 {
			slog.Info("Decoding progress",
				"bytes", totalBytes,
				"chunks", chunkCount)
		}
	}

	slog.Info("Decoding complete",
		"total_bytes", totalBytes,
		"chunks", chunkCount)

	// Write metadata file
	metadata := AudioMetadata{
		SampleRate:    rate,
		Channels:      channels,
		BitsPerSample: bps,
		SourceFile:    inputFile,
		RawFile:       rawFile,
	}

	metaJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		slog.Error("Failed to create metadata", "error", err)
		os.Exit(1)
	}

	err = os.WriteFile(metaFile, metaJSON, 0644)
	if err != nil {
		slog.Error("Failed to write metadata file", "error", err)
		os.Exit(1)
	}

	slog.Info("Metadata saved", "file", metaFile)

	// Print playback instructions
	printPlaybackInstructions(rawFile, rate, channels, bps)
}

func decodeToStdout(inputFile string) {
	// Create decoder
	decoder := flac.NewDecoder()
	defer decoder.Close()

	// Open FLAC file
	err := decoder.Open(inputFile)
	if err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	// Get audio format and log to stderr
	rate, channels, bps := decoder.GetFormat()
	slog.Info("Pipe mode: decoding to stdout",
		"input", inputFile,
		"sample_rate", rate,
		"channels", channels,
		"bits_per_sample", bps)

	channelLayout := "stereo"
	if channels == 1 {
		channelLayout = "mono"
	}

	// Determine PCM format based on bits per sample
	pcmFormat := "s16le"
	if bps == 24 {
		pcmFormat = "s24le"
	} else if bps == 32 {
		pcmFormat = "s32le"
	}

	ffplayCmd := fmt.Sprintf("ffplay -f %s -ar %d -ch_layout %s -", pcmFormat, rate, channelLayout)
	slog.Info("To play, use", "command", ffplayCmd)

	// Decode and write to stdout
	audioSamples := 4 * 1024                                     // Number of samples to decode
	bytesPerSample := bps / 8                                    // Bytes per sample (16, 24, or 32 bits)
	audioBufferBytes := audioSamples * channels * bytesPerSample // Buffer size in bytes
	buffer := make([]byte, audioBufferBytes)

	totalBytes := 0

	for {
		// DecodeSamples expects number of SAMPLES, returns number of SAMPLES
		samplesRead, err := decoder.DecodeSamples(audioSamples, buffer)

		// Check for errors first
		if err != nil {
			break
		}

		// Check if no data was read (EOF)
		if samplesRead == 0 {
			break
		}

		// Calculate bytes to write: samples * channels * bytes_per_sample
		bytesToWrite := samplesRead * channels * bytesPerSample

		// Write to stdout
		written, err := os.Stdout.Write(buffer[:bytesToWrite])
		if err != nil {
			slog.Error("Failed to write to stdout", "error", err)
			os.Exit(1)
		}
		totalBytes += written
	}

	slog.Info("Decoding complete", "total_bytes", totalBytes)
}

func printPlaybackInstructions(rawFile string, rate, channels, bps int) {
	channelLayout := "stereo"
	if channels == 1 {
		channelLayout = "mono"
	}

	// Determine PCM format based on bits per sample
	pcmFormat := "s16le"
	if bps == 24 {
		pcmFormat = "s24le"
	} else if bps == 32 {
		pcmFormat = "s32le"
	}

	ffplayCmd := fmt.Sprintf("ffplay -f %s -ar %d -ch_layout %s %s", pcmFormat, rate, channelLayout, rawFile)
	ffmpegCmd := fmt.Sprintf("ffmpeg -f %s -ar %d -ch_layout %s -i %s output.wav", pcmFormat, rate, channelLayout, rawFile)

	slog.Info("Playback instructions",
		"ffplay", ffplayCmd,
		"ffmpeg", ffmpegCmd)
}
