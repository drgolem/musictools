package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/drgolem/audiokit/pkg/audioplayer"
	"github.com/drgolem/audiokit/pkg/decoder"
	"github.com/drgolem/musictools/internal/decoders"

	"github.com/drgolem/go-portaudio/portaudio"
	"github.com/spf13/cobra"
)

var (
	playDeviceIdx       int
	playBufferCapacity  uint64
	playPAFrames        int
	playSamplesPerFrame int
	playVerbose         bool
)

// playerCmd represents the play command
var playerCmd = &cobra.Command{
	Use:   "play <audio_file>",
	Short: "Play a single audio file",
	Long: `Play a single audio file using PortAudio callback mode with AudioFrameRingBuffer.

Uses the SPSC (Single-Producer Single-Consumer) pattern for efficient audio streaming.

Examples:
  # Play an MP3 file
  musictools play music.mp3

  # Play a FLAC file with specific device
  musictools play -d 0 music.flac

  # Play from stdin (piped WAV)
  musiclab doremi --score scores/greensleeves.csv --stdout | musictools play -

  # Adjust buffer parameters
  musictools play -c 512 -s 2048 music.wav

Supported Formats:
  MP3:    .mp3 (16-bit lossy)
  FLAC:   .flac, .fla (16/24/32-bit lossless)
  WAV:    .wav (8/16/24/32-bit PCM)
  OGG:    .ogg, .oga (Vorbis)
  Opus:   .opus`,
	Args: cobra.ExactArgs(1),
	Run:  runPlayer,
}

func init() {
	rootCmd.AddCommand(playerCmd)

	playerCmd.Flags().IntVarP(&playDeviceIdx, "device", "d", 1, "Audio output device index")
	playerCmd.Flags().Uint64VarP(&playBufferCapacity, "capacity", "c", 256, "Ringbuffer capacity (number of frames)")
	playerCmd.Flags().IntVarP(&playPAFrames, "paframes", "p", 512, "PortAudio frames per buffer")
	playerCmd.Flags().IntVarP(&playSamplesPerFrame, "samples", "s", 4096, "Samples per AudioFrame")
	playerCmd.Flags().BoolVarP(&playVerbose, "verbose", "v", false, "Verbose output (debug logging)")
}

func runPlayer(cmd *cobra.Command, args []string) {
	logLevel := slog.LevelInfo
	if playVerbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	fileName := args[0]

	// Support reading from stdin via "-"
	if fileName == "-" {
		tmpFile, err := os.CreateTemp("", "musictools-stdin-*.wav")
		if err != nil {
			slog.Error("Failed to create temp file for stdin", "error", err)
			os.Exit(1)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := io.Copy(tmpFile, os.Stdin); err != nil {
			tmpFile.Close()
			slog.Error("Failed to read stdin", "error", err)
			os.Exit(1)
		}
		tmpFile.Close()
		fileName = tmpFile.Name()
		slog.Info("Buffered stdin to temp file", "path", fileName)
	} else if _, err := os.Stat(fileName); os.IsNotExist(err) {
		slog.Error("File not found", "path", fileName)
		os.Exit(1)
	}

	slog.Info("Initializing PortAudio")
	if err := portaudio.Initialize(); err != nil {
		slog.Error("Failed to initialize PortAudio", "error", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	slog.Info("PortAudio initialized", "version", portaudio.GetVersion())
	slog.Info("Configuration",
		"device_index", playDeviceIdx,
		"frame_capacity", playBufferCapacity,
		"pa_frames_per_buffer", playPAFrames,
		"samples_per_audioframe", playSamplesPerFrame)

	player := audioplayer.New(playDeviceIdx, playBufferCapacity, playPAFrames, playSamplesPerFrame)

	slog.Info("Opening audio file", "path", fileName)
	dec, err := safeNewDecoder(fileName)
	if err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	player.SetDecoder(dec, filepath.Base(fileName))

	if err := player.Play(); err != nil {
		slog.Error("Failed to start playback", "error", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	statusDone := make(chan struct{})
	go monitorPlayback(player, statusDone)

	done := make(chan struct{})
	go func() {
		player.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Playback completed")
	case sig := <-sigChan:
		slog.Info("Signal received, stopping", "signal", sig)
	}

	close(statusDone)
	if err := player.Stop(); err != nil {
		slog.Error("Failed to stop player", "error", err)
	}

	slog.Info("Exiting")
}

// safeNewDecoder wraps decoders.NewDecoder with panic recovery.
// go-riff panics on truncated/invalid WAV files instead of returning an error.
func safeNewDecoder(fileName string) (dec decoder.AudioDecoder, err error) {
	defer func() {
		if r := recover(); r != nil {
			dec = nil
			err = fmt.Errorf("failed to decode file (possibly corrupt or truncated): %v", r)
		}
	}()
	return decoders.NewDecoder(fileName)
}
