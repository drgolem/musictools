package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"musictools/pkg/audioplayer"

	"github.com/drgolem/go-portaudio/portaudio"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Define flags
	deviceIdx := flag.Int("device", 1, "Audio output device index (default: 1)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: play [options] <audio_file>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Plays an MP3 or FLAC file using producer/consumer pattern with ringbuffer")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  play music.mp3")
		fmt.Fprintln(os.Stderr, "  play -device 0 music.flac")
		fmt.Fprintln(os.Stderr, "  play -device 2 music.mp3")
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	fileName := flag.Arg(0)

	// Initialize PortAudio
	slog.Info("Initializing PortAudio")
	if err := portaudio.Initialize(); err != nil {
		slog.Error("Failed to initialize PortAudio", "error", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	slog.Info("PortAudio initialized", "version", portaudio.GetVersion())
	slog.Info("Using audio output device", "device_index", *deviceIdx)

	// Create player with custom device
	config := audioplayer.DefaultConfig()
	config.DeviceIndex = *deviceIdx
	player := audioplayer.NewPlayer(config)

	// Open audio file
	slog.Info("Opening file", "path", fileName)
	if err := player.OpenFile(fileName); err != nil {
		slog.Error("Failed to open file", "error", err)
		os.Exit(1)
	}

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start playback
	if err := player.Play(); err != nil {
		slog.Error("Failed to start playback", "error", err)
		os.Exit(1)
	}

	// Monitor buffer status
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				available, size := player.GetBufferStatus()
				percentage := float64(available) / float64(size) * 100
				slog.Info("Buffer status",
					"available_bytes", available,
					"buffer_size", size,
					"fill_percentage", fmt.Sprintf("%.1f%%", percentage))
			case <-sigChan:
				return
			}
		}
	}()

	// Wait for playback to complete or interrupt
	done := make(chan struct{})
	go func() {
		player.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Playback completed")
	case <-sigChan:
		slog.Info("Interrupt received, stopping playback")
		if err := player.Stop(); err != nil {
			slog.Error("Failed to stop player", "error", err)
		}
	}

	slog.Info("Exiting")
}
