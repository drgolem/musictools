package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"learnRingbuffer/internal/fileplayer"
	"learnRingbuffer/pkg/types"

	"github.com/drgolem/go-portaudio/portaudio"
	"github.com/spf13/cobra"
)

var (
	// Flags for playlist command
	playlistDeviceIdx       int
	playlistBufferCapacity  uint64
	playlistPAFrames        int
	playlistSamplesPerFrame int
	playlistVerbose         bool
)

// playlistCmd represents the playlist command
var playlistCmd = &cobra.Command{
	Use:   "playlist <audio_file> [audio_file...]",
	Short: "Play multiple audio files sequentially",
	Long: `Play multiple audio files one after another using PortAudio callback mode.

This command plays a list of audio files sequentially, closing and reinitializing
the audio stream between files. It uses the AudioFrameRingBuffer for efficient
frame-based audio streaming with the SPSC (Single-Producer Single-Consumer) pattern.

Examples:
  # Play multiple files
  learnRingbuffer playlist song1.mp3 song2.flac song3.wav

  # Play all MP3 files in current directory
  learnRingbuffer playlist *.mp3

  # Use specific device with verbose output
  learnRingbuffer playlist -d 0 -v music/*.flac

  # Adjust buffer parameters
  learnRingbuffer playlist -c 512 -s 2048 *.wav

Supported Formats:
  MP3:  .mp3 (16-bit lossy)
  FLAC: .flac, .fla (16/24/32-bit lossless)
  WAV:  .wav (8/16/24/32-bit PCM)`,
	Args: cobra.MinimumNArgs(1),
	Run:  runPlaylist,
}

func init() {
	rootCmd.AddCommand(playlistCmd)

	playlistCmd.Flags().IntVarP(&playlistDeviceIdx, "device", "d", 1, "Audio output device index")
	playlistCmd.Flags().Uint64VarP(&playlistBufferCapacity, "capacity", "c", 256, "Ringbuffer capacity (number of frames)")
	playlistCmd.Flags().IntVarP(&playlistPAFrames, "paframes", "p", 512, "PortAudio frames per buffer")
	playlistCmd.Flags().IntVarP(&playlistSamplesPerFrame, "samples", "s", 4096, "Samples per AudioFrame")
	playlistCmd.Flags().BoolVarP(&playlistVerbose, "verbose", "v", false, "Verbose output (debug logging)")
}

func runPlaylist(cmd *cobra.Command, args []string) {
	logLevel := slog.LevelInfo
	if playlistVerbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	files := args

	slog.Info("Initializing PortAudio")
	if err := portaudio.Initialize(); err != nil {
		slog.Error("Failed to initialize PortAudio", "error", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	slog.Info("PortAudio initialized", "version", portaudio.GetVersion())
	slog.Info("Configuration",
		"device_index", playlistDeviceIdx,
		"frame_capacity", playlistBufferCapacity,
		"pa_frames_per_buffer", playlistPAFrames,
		"samples_per_audioframe", playlistSamplesPerFrame,
		"file_count", len(files))

	player := fileplayer.NewFilePlayer(playlistDeviceIdx, playlistBufferCapacity, playlistPAFrames, playlistSamplesPerFrame)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	interrupted := false

	for i, fileName := range files {
		if interrupted {
			break
		}

		slog.Info("Playing file", "index", i+1, "total", len(files), "file", fileName)

		if err := player.OpenFile(fileName); err != nil {
			slog.Error("Failed to open file", "file", fileName, "error", err)
			continue
		}

		if err := player.PlayFile(); err != nil {
			slog.Error("Failed to start playback", "file", fileName, "error", err)
			continue
		}

		statusDone := make(chan struct{})
		go monitorPlayback(player, statusDone)

		done := make(chan struct{})
		go func() {
			player.Wait()
			close(done)
		}()

		select {
		case <-done:
			slog.Info("File completed", "file", fileName)
			close(statusDone)
			if err := player.Stop(); err != nil {
				slog.Error("Failed to stop player", "error", err)
			}
		case sig := <-sigChan:
			slog.Info("Signal received, stopping", "signal", sig)
			interrupted = true
			close(statusDone)
			if err := player.Stop(); err != nil {
				slog.Error("Failed to stop player", "error", err)
			}
		}
	}

	if interrupted {
		slog.Info("Playback interrupted")
	} else {
		slog.Info("All files completed", "total", len(files))
	}

	slog.Info("Exiting")
}

// monitorPlayback monitors and logs playback status every 2 seconds for any PlaybackMonitor
func monitorPlayback(monitor types.PlaybackMonitor, done chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status := monitor.GetPlaybackStatus()

			// Calculate played audio time from samples (actually sent to speakers)
			playedTimeSeconds := float64(status.PlayedSamples) / float64(status.SampleRate)

			// Calculate buffered audio time (decoded but not yet played)
			bufferedTimeSeconds := float64(status.BufferedSamples) / float64(status.SampleRate)

			// Format elapsed time as hh:mm:ss.msec
			totalMilliseconds := status.ElapsedTime.Milliseconds()
			hours := totalMilliseconds / 3600000
			minutes := (totalMilliseconds % 3600000) / 60000
			seconds := (totalMilliseconds % 60000) / 1000
			milliseconds := totalMilliseconds % 1000
			elapsedStr := fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)

			// Format played time as hh:mm:ss.msec (same format as elapsed)
			playedMilliseconds := int64(playedTimeSeconds * 1000)
			playedHours := playedMilliseconds / 3600000
			playedMinutes := (playedMilliseconds % 3600000) / 60000
			playedSeconds := (playedMilliseconds % 60000) / 1000
			playedMsec := playedMilliseconds % 1000
			playedTimeStr := fmt.Sprintf("%02d:%02d:%02d.%03d", playedHours, playedMinutes, playedSeconds, playedMsec)

			bufferedTimeStr := fmt.Sprintf("%.3fs", bufferedTimeSeconds)

			formatStr := fmt.Sprintf("%d:%d:%d",
				status.SampleRate, status.BitsPerSample, status.Channels)

			portAudioStr := fmt.Sprintf("%dHz:%dbit:%dch:%dframes",
				status.SampleRate, status.BitsPerSample, status.Channels, status.FramesPerBuffer)

			slog.Info("Playback status",
				"file", status.FileName,
				"format", formatStr,
				"portaudio", portAudioStr,
				"played", playedTimeStr,
				"buffered", bufferedTimeStr,
				"elapsed", elapsedStr)
		case <-done:
			return
		}
	}
}
