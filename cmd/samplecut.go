package cmd

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/drgolem/audiokit/pkg/decoder"
	"github.com/drgolem/musictools/internal/decoders"
	"github.com/spf13/cobra"
	"github.com/youpy/go-wav"
)

var samplecutCmd = &cobra.Command{
	Use:   "samplecut",
	Short: "Cut a segment from an audio file",
	Long:  `Cut a time segment from an audio file and save as WAV.`,
	Run:   doSamplecutCmd,
}

func init() {
	rootCmd.AddCommand(samplecutCmd)

	samplecutCmd.Flags().String("in", "", "file to cut")
	samplecutCmd.Flags().String("out", "out_cut.wav", "output wav file")
	samplecutCmd.Flags().String("start", "10s5ms", "start")
	samplecutCmd.Flags().String("duration", "30s", "duration")
}

func doSamplecutCmd(cmd *cobra.Command, args []string) {
	inFileName, err := cmd.Flags().GetString("in")
	if err != nil {
		slog.Error("failed to get flag", "error", err)
		return
	}
	if _, err := os.Stat(inFileName); os.IsNotExist(err) {
		slog.Error("path does not exist", "path", inFileName)
		return
	}
	outFileName, err := cmd.Flags().GetString("out")
	if err != nil {
		slog.Error("failed to get flag", "error", err)
		return
	}

	startStr, err := cmd.Flags().GetString("start")
	if err != nil {
		slog.Error("failed to get flag", "error", err)
		return
	}
	start, err := time.ParseDuration(startStr)
	if err != nil {
		slog.Error("invalid start time", "value", startStr, "error", err)
		return
	}

	durtStr, err := cmd.Flags().GetString("duration")
	if err != nil {
		slog.Error("failed to get flag", "error", err)
		return
	}
	dur, err := time.ParseDuration(durtStr)
	if err != nil {
		slog.Error("invalid duration", "value", durtStr, "error", err)
		return
	}

	dec, err := decoders.NewDecoder(inFileName)
	if err != nil {
		slog.Error("failed to create decoder", "error", err)
		return
	}
	defer dec.Close()

	sampleRate, channels, bitsPerSample := dec.GetFormat()

	slog.Info("Samplecut",
		"input", inFileName,
		"output", outFileName,
		"start", start,
		"duration", dur,
		"sample_rate", sampleRate,
		"channels", channels,
		"bits_per_sample", bitsPerSample)

	startSamples := int(start.Seconds() * float64(sampleRate))
	durationSamples := int(dur.Seconds() * float64(sampleRate))
	bytesPerFrame := channels * bitsPerSample / 8

	// Seek to start position
	if startSamples > 0 {
		if seekable, ok := dec.(decoder.Seekable); ok {
			if _, err := seekable.Seek(int64(startSamples), io.SeekCurrent); err != nil {
				slog.Error("seek failed", "error", err)
				return
			}
		} else {
			// Skip samples by decoding and discarding
			skipped := 0
			skipBuf := make([]byte, 2048*bytesPerFrame)
			for skipped < startSamples {
				toRead := min(2048, startSamples-skipped)
				n, err := dec.DecodeSamples(toRead, skipBuf)
				if err != nil || n == 0 {
					slog.Error("failed to skip to start position", "skipped", skipped, "target", startSamples)
					return
				}
				skipped += n
			}
		}
	}

	// Read duration samples
	audioData := make([]byte, 0, durationSamples*bytesPerFrame)
	readBuf := make([]byte, 2048*bytesPerFrame)
	samplesRead := 0

	for samplesRead < durationSamples {
		toRead := min(2048, durationSamples-samplesRead)
		n, err := dec.DecodeSamples(toRead, readBuf)
		if n > 0 {
			audioData = append(audioData, readBuf[:n*bytesPerFrame]...)
			samplesRead += n
		}
		if err != nil || n == 0 {
			break
		}
	}

	slog.Info("Decoded segment", "samples", samplesRead)

	writeWav(outFileName, audioData, samplesRead, channels, sampleRate, bitsPerSample)
}

func writeWav(outFileName string, audioData []byte, samplesCnt, channels, sampleRate, bitsPerSample int) {
	fOut, err := os.OpenFile(outFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		slog.Error("failed to create output file", "error", err)
		return
	}
	defer fOut.Close()

	wavWriter := wav.NewWriter(fOut,
		uint32(samplesCnt),
		uint16(channels),
		uint32(sampleRate),
		uint16(bitsPerSample))

	if _, err := wavWriter.Write(audioData); err != nil {
		slog.Error("failed to write WAV data", "error", err)
		return
	}
	slog.Info("WAV written", "file", outFileName)
}
