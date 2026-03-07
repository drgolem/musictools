package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "musictools",
	Short: "Audio player and converter",
	Long: `musictools - Command-line audio player and converter.

Supports MP3, FLAC, WAV, OGG Vorbis, and Opus formats.

Commands:
  play       Play a single audio file
  playlist   Play multiple files sequentially
  transform  Resample and convert to WAV
  samplecut  Extract a time segment from an audio file`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
