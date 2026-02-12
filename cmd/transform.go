package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/drgolem/musictools/pkg/decoders"
	"github.com/drgolem/musictools/pkg/types"

	"github.com/spf13/cobra"
	wav "github.com/youpy/go-wav"
	soxr "github.com/zaf/resample"
)

var transformCmd = &cobra.Command{
	Use:   "transform <input_file>",
	Short: "Transform audio file sample rate and format",
	Long: `Transform audio files to different sample rates and convert to WAV format.
Supports input from MP3, FLAC, and WAV formats with optional mono conversion.

Examples:
  # Transform MP3 to 48kHz WAV
  musictools transform input.mp3 --new-samplerate 48000 --out output.wav

  # Transform FLAC to 44.1kHz mono WAV
  musictools transform input.flac --new-samplerate 44100 --mono --out output.wav

  # Transform WAV with default settings (48kHz)
  musictools transform input.wav

Supported Input Formats:
  - MP3 (.mp3)
  - FLAC (.flac)
  - WAV (.wav)

Output Format:
  - WAV (16-bit PCM)

Sample Rate Options:
  Common rates: 8000, 16000, 22050, 44100, 48000, 96000, 192000 Hz`,
	Args: cobra.ExactArgs(1),
	Run:  runTransform,
}

func init() {
	rootCmd.AddCommand(transformCmd)

	transformCmd.Flags().Int("new-samplerate", 48000, "Target sample rate in Hz")
	transformCmd.Flags().String("out", "out_transformed.wav", "Output WAV file path")
	transformCmd.Flags().Bool("mono", false, "Convert output to mono signal (average channels)")
}

func runTransform(cmd *cobra.Command, args []string) {
	inFileName := args[0]

	if _, err := os.Stat(inFileName); os.IsNotExist(err) {
		slog.Error("Input file not found", "path", inFileName)
		os.Exit(1)
	}

	newSampleRate, err := cmd.Flags().GetInt("new-samplerate")
	if err != nil {
		slog.Error("Failed to get new-samplerate flag", "error", err)
		os.Exit(1)
	}

	outFileName, err := cmd.Flags().GetString("out")
	if err != nil {
		slog.Error("Failed to get out flag", "error", err)
		os.Exit(1)
	}

	convertToMono, err := cmd.Flags().GetBool("mono")
	if err != nil {
		slog.Error("Failed to get mono flag", "error", err)
		os.Exit(1)
	}

	if newSampleRate <= 0 || newSampleRate > 384000 {
		slog.Error("Invalid sample rate", "rate", newSampleRate, "valid_range", "1-384000")
		os.Exit(1)
	}

	decoder, err := decoders.NewDecoder(inFileName)
	if err != nil {
		slog.Error("Failed to create decoder", "error", err)
		os.Exit(1)
	}
	defer decoder.Close()

	inSampleRate, channels, bitsPerSample := decoder.GetFormat()

	slog.Info("Audio transformation starting",
		"input_file", inFileName,
		"input_sample_rate", inSampleRate,
		"input_channels", channels,
		"input_bits_per_sample", bitsPerSample,
		"output_sample_rate", newSampleRate,
		"output_mono", convertToMono,
		"output_file", outFileName)

	slog.Info("Decoding audio data")
	audioData, totalSamples, err := decodeAllAudio(decoder, channels, bitsPerSample)
	if err != nil {
		slog.Error("Failed to decode audio", "error", err)
		os.Exit(1)
	}

	slog.Info("Decoding complete",
		"input_samples", totalSamples,
		"input_bytes", len(audioData))

	slog.Info("Resampling audio",
		"from_rate", inSampleRate,
		"to_rate", newSampleRate)

	resampledData, err := resampleAudio(audioData, inSampleRate, newSampleRate, channels)
	if err != nil {
		slog.Error("Failed to resample audio", "error", err)
		os.Exit(1)
	}

	bytesPerSample := bitsPerSample / 8
	outSamples := len(resampledData) / (channels * bytesPerSample)

	slog.Info("Resampling complete",
		"output_samples", outSamples,
		"output_bytes", len(resampledData))

	outChannels := channels
	outputData := resampledData

	if convertToMono && channels > 1 {
		slog.Info("Converting to mono", "input_channels", channels)
		outputData = convertToMono16Bit(resampledData, channels)
		outChannels = 1
		slog.Info("Mono conversion complete", "output_channels", 1)
	}

	slog.Info("Writing output WAV file", "path", outFileName)
	if err := writeWAVFile(outFileName, outputData, uint32(outSamples), uint16(outChannels), uint32(newSampleRate), uint16(bitsPerSample)); err != nil {
		slog.Error("Failed to write WAV file", "error", err)
		os.Exit(1)
	}

	slog.Info("Transformation complete",
		"input_samples", totalSamples,
		"output_samples", outSamples,
		"sample_rate_ratio", fmt.Sprintf("%.3f", float64(newSampleRate)/float64(inSampleRate)))
}

// decodeAllAudio reads all audio data from the decoder into memory
func decodeAllAudio(decoder types.AudioDecoder, channels, bitsPerSample int) ([]byte, int, error) {
	const bufferSamples = 4096
	bytesPerSample := bitsPerSample / 8
	bufferSize := bufferSamples * channels * bytesPerSample

	buffer := make([]byte, bufferSize)
	audioData := make([]byte, 0, bufferSize*10) // Pre-allocate for efficiency
	totalSamples := 0

	for {
		samplesRead, err := decoder.DecodeSamples(bufferSamples, buffer)
		if samplesRead > 0 {
			bytesRead := samplesRead * channels * bytesPerSample
			audioData = append(audioData, buffer[:bytesRead]...)
			totalSamples += samplesRead
		}

		if err != nil {
			// Check if it's EOF (expected at end of file)
			if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "done") {
				break
			}
			return nil, 0, fmt.Errorf("decode error: %w", err)
		}

		if samplesRead == 0 {
			break
		}
	}

	return audioData, totalSamples, nil
}

// resampleAudio resamples audio data using SoXR (high-quality resampler)
func resampleAudio(audioData []byte, fromRate, toRate, channels int) ([]byte, error) {
	if fromRate == toRate {
		return audioData, nil
	}

	var bufResampled bytes.Buffer
	bufWriter := bufio.NewWriter(&bufResampled)

	resampler, err := soxr.New(
		bufWriter,
		float64(fromRate),
		float64(toRate),
		channels,
		soxr.I16,    // 16-bit input
		soxr.HighQ,  // High quality
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resampler: %w", err)
	}

	_, err = resampler.Write(audioData)
	if err != nil {
		resampler.Close()
		return nil, fmt.Errorf("failed to resample: %w", err)
	}

	if err := resampler.Close(); err != nil {
		return nil, fmt.Errorf("failed to close resampler: %w", err)
	}

	if err := bufWriter.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush buffer: %w", err)
	}

	return bufResampled.Bytes(), nil
}

// convertToMono16Bit converts stereo (or multi-channel) 16-bit audio to mono by averaging channels
func convertToMono16Bit(stereoData []byte, channels int) []byte {
	if channels == 1 {
		return stereoData
	}

	monoSize := len(stereoData) / channels
	monoData := make([]byte, monoSize)

	idx := 0
	outIdx := 0

	for idx < len(stereoData) {
		sum := int32(0)
		for ch := 0; ch < channels; ch++ {
			if idx+1 >= len(stereoData) {
				break
			}

			// Read 16-bit sample (little-endian)
			b0 := int16(stereoData[idx])
			b1 := int16(stereoData[idx+1])
			sample := int16((b1 << 8) | b0)

			sum += int32(sample)
			idx += 2
		}

		// Average channels
		avgSample := int16(sum / int32(channels))

		// Write mono sample (16-bit little-endian)
		if outIdx+1 < len(monoData) {
			monoData[outIdx] = byte(avgSample & 0xFF)
			monoData[outIdx+1] = byte((avgSample >> 8) & 0xFF)
			outIdx += 2
		}
	}

	return monoData
}

// writeWAVFile writes audio data to a WAV file
func writeWAVFile(fileName string, audioData []byte, numSamples uint32, numChannels uint16, sampleRate uint32, bitsPerSample uint16) error {
	fOut, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer fOut.Close()

	wavWriter := wav.NewWriter(fOut, numSamples, numChannels, sampleRate, bitsPerSample)

	if _, err := wavWriter.Write(audioData); err != nil {
		return fmt.Errorf("failed to write WAV data: %w", err)
	}

	return nil
}
