package decoders

import (
	"github.com/drgolem/audiokit/pkg/decoder"
	"github.com/drgolem/audiokit/pkg/decoder/flac"
	"github.com/drgolem/audiokit/pkg/decoder/mp3"
	"github.com/drgolem/audiokit/pkg/decoder/opus"
	"github.com/drgolem/audiokit/pkg/decoder/vorbis"
	"github.com/drgolem/audiokit/pkg/decoder/wav"
)

// NewRegistry creates a decoder registry pre-loaded with all supported codecs.
func NewRegistry() *decoder.Registry {
	r := decoder.NewRegistry()
	r.Register(".mp3", func(int) (decoder.AudioDecoder, error) { return mp3.NewDecoder(), nil })
	r.Register(".flac", func(bps int) (decoder.AudioDecoder, error) { return flac.NewDecoder(bps) })
	r.Register(".fla", func(bps int) (decoder.AudioDecoder, error) { return flac.NewDecoder(bps) })
	r.Register(".wav", func(int) (decoder.AudioDecoder, error) { return wav.NewDecoder(), nil })
	r.Register(".ogg", func(bps int) (decoder.AudioDecoder, error) { return vorbis.NewDecoder(bps) })
	r.Register(".oga", func(bps int) (decoder.AudioDecoder, error) { return vorbis.NewDecoder(bps) })
	r.Register(".opus", func(int) (decoder.AudioDecoder, error) { return opus.NewDecoder(), nil })
	return r
}

// NewDecoder creates and opens the appropriate decoder based on file extension.
// Supports .mp3, .flac, .fla, .wav, .ogg, .oga, and .opus formats.
func NewDecoder(fileName string) (decoder.AudioDecoder, error) {
	return NewRegistry().NewFromFile(fileName, 0)
}
