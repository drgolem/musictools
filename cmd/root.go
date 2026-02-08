package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "learnRingbuffer",
	Short: "Lock-free SPSC ringbuffer audio player",
	Long: `learnRingbuffer - A production-ready audio player demonstrating lock-free
SPSC (Single-Producer Single-Consumer) ringbuffer implementation for real-time
audio streaming.

Features:
  - Lock-free SPSC ringbuffer with zero-copy audio processing
  - Producer/consumer architecture for real-time streaming
  - Support for MP3, FLAC, and WAV audio formats
  - Configurable buffer sizes and audio devices
  - Thread-safe implementation with comprehensive safety analysis
  - Sample rate transformation and format conversion

Commands:
  - play: Play audio files with real-time monitoring
  - transform: Convert audio files to different sample rates and WAV format`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
