package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drgolem/musictools/pkg/audioplayer"
	"github.com/drgolem/musictools/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
	"github.com/spf13/cobra"
)

const (
	version = "1.0.0"
)

var (
	deviceIdx   int
	bufferSize  uint64
	frames      int
	showVersion bool
	verbose     bool
)

// playerCmd represents the player command
var playerCmd = &cobra.Command{
	Use:   "play <audio_file>",
	Short: "Play audio files (MP3, FLAC, WAV)",
	Long: `High-performance audio player using lock-free ringbuffer and producer/consumer pattern.
Supports MP3, FLAC, and WAV formats with real-time status reporting.

Examples:
  # Play an MP3 file
  musictools play music.mp3

  # Play a FLAC file with specific device
  musictools play -device 0 music.flac

  # Play a WAV file
  musictools play audio.wav

  # Use larger buffer for better stability
  musictools play -buffer 524288 music.mp3

  # Lower latency with smaller buffer
  musictools play -buffer 65536 -frames 256 music.flac

Buffer Recommendations:
  Low latency:    -buffer 65536  -frames 256   (lower CPU usage tolerance)
  Balanced:       -buffer 262144 -frames 512   (default, recommended)
  High stability: -buffer 524288 -frames 1024  (high CPU load scenarios)

Supported Formats:
  MP3:  .mp3 (16-bit lossy)
  FLAC: .flac (16/24/32-bit lossless)
  WAV:  .wav (8/16/24/32-bit PCM)

Status Reporting:
  Playback status is displayed every 2 seconds showing:
  - File name and audio format
  - Elapsed samples and audio time
  - Real-time elapsed time`,
	Args: cobra.ExactArgs(1),
	Run:  runPlayer,
}

func init() {
	rootCmd.AddCommand(playerCmd)

	playerCmd.Flags().IntVarP(&deviceIdx, "device", "d", 1, "Audio output device index")
	playerCmd.Flags().Uint64VarP(&bufferSize, "buffer", "b", 256*1024, "Ringbuffer size in bytes (power of 2)")
	playerCmd.Flags().IntVarP(&frames, "frames", "f", 512, "Audio frames per buffer")
	playerCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output (debug logging)")
	playerCmd.Flags().BoolVar(&showVersion, "version", false, "Show version information")
}

func runPlayer(cmd *cobra.Command, args []string) {
	// Handle version flag
	if showVersion {
		fmt.Printf("Audio Player v%s\n", version)
		fmt.Println("Built with:")
		fmt.Println("  - Lock-free SPSC ringbuffer")
		fmt.Println("  - Producer/consumer architecture")
		fmt.Println("  - Zero-copy audio streaming")
		fmt.Println("  - PortAudio for cross-platform audio")
		os.Exit(0)
	}

	fileName := args[0]

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		slog.Error("File not found", "path", fileName)
		os.Exit(1)
	}

	slog.Info("Initializing PortAudio")
	if err := portaudio.Initialize(); err != nil {
		slog.Error("Failed to initialize PortAudio", "error", err)
		slog.Error("Hint: Make sure PortAudio is installed on your system")
		os.Exit(1)
	}
	defer portaudio.Terminate()

	slog.Info("PortAudio initialized",
		"version", portaudio.GetVersion())
	slog.Info("Audio configuration",
		"device_index", deviceIdx,
		"buffer_size", bufferSize,
		"frames_per_buffer", frames)

	config := audioplayer.Config{
		BufferSize:      bufferSize,
		FramesPerBuffer: frames,
		DeviceIndex:     deviceIdx,
	}
	player := audioplayer.NewPlayer(config)

	slog.Info("Opening audio file", "path", fileName)
	if err := player.OpenFile(fileName); err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	slog.Info("Starting playback")
	if err := player.Play(); err != nil {
		slog.Error("Failed to start playback", "error", err)
		os.Exit(1)
	}

	statusDone := make(chan struct{})
	go monitorPlayback(&playerMonitorAdapter{player: player}, statusDone)

	var monitorDone chan struct{}
	if verbose {
		monitorDone = make(chan struct{})
		go monitorBufferStatus(player, monitorDone)
	}

	done := make(chan struct{})
	go func() {
		player.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Playback completed successfully")
	case sig := <-sigChan:
		slog.Info("Signal received, stopping playback", "signal", sig)
		if err := player.Stop(); err != nil {
			slog.Error("Failed to stop player", "error", err)
		}
	}

	close(statusDone)
	if verbose && monitorDone != nil {
		close(monitorDone)
	}

	slog.Info("Exiting")
}

// playerMonitorAdapter adapts audioplayer.Player to the types.PlaybackMonitor interface
type playerMonitorAdapter struct {
	player *audioplayer.Player
}

// GetPlaybackStatus implements types.PlaybackMonitor for audioplayer.Player
func (a *playerMonitorAdapter) GetPlaybackStatus() types.PlaybackStatus {
	// audioplayer.Player now returns types.PlaybackStatus directly, so just return it
	return a.player.GetPlaybackStatus()
}

// monitorBufferStatus monitors and logs ringbuffer status
func monitorBufferStatus(player *audioplayer.Player, done chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			available, size := player.GetBufferStatus()
			percentage := float64(available) / float64(size) * 100

			level := slog.LevelInfo
			if percentage < 25 {
				level = slog.LevelWarn
			}

			slog.Log(nil, level, "Buffer status",
				"available_bytes", available,
				"buffer_size", size,
				"fill_percentage", fmt.Sprintf("%.1f%%", percentage))

			if percentage < 10 {
				slog.Warn("Buffer critically low - possible underruns")
			}
		case <-done:
			return
		}
	}
}
