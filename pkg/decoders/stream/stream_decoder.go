package stream

import (
	"context"
	"sync"
)

// AudioFormat describes the audio stream format
type AudioFormat struct {
	SampleRate     int
	Channels       int
	BytesPerSample int
}

// AudioPacket represents a chunk of decoded audio data
type AudioPacket struct {
	Audio        []byte
	SamplesCount int
	Format       AudioFormat
}

// AudioPacketProvider is the interface for sources that provide audio data
// This allows musictools to play from any source: network streams, buffers, etc.
type AudioPacketProvider interface {
	// ReadAudioPacket reads the next audio packet
	// Returns the packet and any error (io.EOF when stream ends)
	ReadAudioPacket(ctx context.Context, samples int) (*AudioPacket, error)
}

// StreamDecoder implements AudioDecoder for streaming audio sources
// This allows musictools to play audio from any source that can provide audio packets
type StreamDecoder struct {
	provider     AudioPacketProvider
	format       AudioFormat
	formatMx     sync.RWMutex
	formatChange chan AudioFormat
	ctx          context.Context
}

// NewStreamDecoder creates a decoder for streaming audio sources
func NewStreamDecoder(ctx context.Context, provider AudioPacketProvider, initialFormat AudioFormat) *StreamDecoder {
	return &StreamDecoder{
		provider:     provider,
		format:       initialFormat,
		formatChange: make(chan AudioFormat, 1),
		ctx:          ctx,
	}
}

func (d *StreamDecoder) Open(fileName string) error {
	// No-op for stream decoder - already initialized
	return nil
}

func (d *StreamDecoder) Close() error {
	return nil
}

func (d *StreamDecoder) GetFormat() (rate, channels, bitsPerSample int) {
	d.formatMx.RLock()
	defer d.formatMx.RUnlock()
	return d.format.SampleRate,
		d.format.Channels,
		d.format.BytesPerSample * 8
}

func (d *StreamDecoder) DecodeSamples(samples int, audio []byte) (int, error) {
	pkt, err := d.provider.ReadAudioPacket(d.ctx, samples)
	if err != nil {
		return 0, err
	}

	if pkt.SamplesCount == 0 {
		return 0, nil // No data available
	}

	// Check for format change
	if d.formatChanged(pkt.Format) {
		d.formatMx.Lock()
		d.format = pkt.Format
		d.formatMx.Unlock()

		// Signal format change
		select {
		case d.formatChange <- pkt.Format:
		default:
		}
	}

	// Copy audio data
	bytesToCopy := pkt.SamplesCount * pkt.Format.Channels * pkt.Format.BytesPerSample
	copy(audio, pkt.Audio[:bytesToCopy])

	return pkt.SamplesCount, nil
}

func (d *StreamDecoder) formatChanged(newFormat AudioFormat) bool {
	d.formatMx.RLock()
	defer d.formatMx.RUnlock()

	return d.format.SampleRate != newFormat.SampleRate ||
		d.format.Channels != newFormat.Channels ||
		d.format.BytesPerSample != newFormat.BytesPerSample
}

// FormatChanges returns a channel that receives format change notifications
func (d *StreamDecoder) FormatChanges() <-chan AudioFormat {
	return d.formatChange
}
