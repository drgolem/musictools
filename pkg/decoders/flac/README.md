# FLAC Decoder

**📖 For common API documentation, usage patterns, and integration examples, see [Common Decoders Documentation](../README.md)**

A Go package for decoding FLAC files using the [go-flac](https://github.com/drgolem/go-flac) library, which provides high-quality lossless audio decoding.

## FLAC-Specific Features

- **Lossless compression**: Perfect audio quality with no data loss
- **High bit depth support**: 16-bit, 24-bit, and 32-bit samples
- **High sample rates**: Up to 655,350 Hz (typical studio: 48kHz, 96kHz, 192kHz)
- **Better metadata**: Rich tag support and cover art
- **Compression ratio**: 40-60% of uncompressed WAV size

## Installation

This package requires the **FLAC** library to be installed on your system.

```bash
go get github.com/drgolem/go-flac
```

### macOS
```bash
brew install flac
```

### Ubuntu/Debian
```bash
sudo apt-get install libflac-dev
```

### Fedora/RHEL
```bash
sudo dnf install flac-devel
```

## Bit Depth Support

FLAC supports multiple bit depths, preserving the original audio quality:

| Bit Depth | Dynamic Range | Use Case |
|-----------|---------------|----------|
| **16-bit** | 96 dB | CD quality, general listening |
| **24-bit** | 144 dB | Studio recording, mastering |
| **32-bit** | 192 dB | Professional audio, archival |

**Note:** The decoder automatically handles the bit depth conversion. Use `GetFormat()` to determine the bit depth of the source file.

```go
rate, channels, bitsPerSample := decoder.GetFormat()
fmt.Printf("Bit depth: %d-bit\n", bitsPerSample)
```

## Common FLAC Audio Formats

| Sample Rate | Channels | Bits | Description |
|-------------|----------|------|-------------|
| 44100 Hz | 2 | 16 | CD quality stereo |
| 48000 Hz | 2 | 16 | Professional audio stereo |
| 48000 Hz | 2 | 24 | Studio quality stereo |
| 96000 Hz | 2 | 24 | High-resolution audio |
| 192000 Hz | 2 | 24 | Audiophile quality |

## FLAC vs MP3

Understanding the differences helps choose the right format:

| Feature | FLAC | MP3 |
|---------|------|-----|
| **Compression** | Lossless | Lossy |
| **Quality** | Perfect (original) | Good (data loss) |
| **Bit Depth** | 16/24/32-bit | 16-bit only |
| **Sample Rates** | Up to 655,350 Hz | Typically 44.1-48 kHz |
| **File Size** | 40-60% of WAV | 10-15% of WAV |
| **Use Case** | Archival, hi-fi, studio | Streaming, portable devices |
| **Decode Speed** | Medium | Medium |
| **CPU Usage** | Low-Medium | Low |

**When to use FLAC:**
- Archiving music collections
- Studio recording and mastering
- Hi-fi audio systems
- When storage space is not critical

**When to use MP3:**
- Streaming over networks
- Portable devices with limited storage
- Podcasts and spoken word
- When quality trade-off is acceptable

## Example: Decode FLAC to Raw PCM

The package includes a command-line tool to decode FLAC files:

```bash
# Build the decoder example
cd pkg/decoders/flac/examples/decode
go build -o decode main.go

# Decode a FLAC file to files
./decode music.flac

# Pipe directly to ffplay (no intermediate files!)
./decode music.flac --pipe | ffplay -f s16le -ar 44100 -ch_layout stereo -

# High-quality 24-bit playback
./decode hires.flac --pipe | ffplay -f s24le -ar 96000 -ch_layout stereo -

# Stream and convert to WAV
./decode music.flac --pipe | ffmpeg -f s16le -ar 44100 -ch_layout stereo -i - output.wav
```

### Output Files

When saving to files (file mode), the tool creates:
- `output.raw` - Raw PCM audio data (bit depth matches source)
- `output.meta` - JSON metadata with format information

**Metadata format:**
```json
{
  "sample_rate": 44100,
  "channels": 2,
  "bits_per_sample": 16,
  "source_file": "music.flac",
  "raw_file": "output.raw"
}
```

## FLAC-Specific API Methods

### `func (d *Decoder) BitsPerSample() int`

Returns the bits per sample for the FLAC file. This is FLAC-specific and provides the actual bit depth of the audio.

```go
bitsPerSample := decoder.BitsPerSample()
fmt.Printf("Audio bit depth: %d-bit\n", bitsPerSample)
```

## Performance

The decoder uses the optimized **FLAC reference library**, which provides:
- **Fast decoding**: 100x+ real-time on modern systems
- **SIMD optimizations**: Platform-specific optimizations where available
- **Low CPU usage**: Efficient streaming decoder
- **Minimal memory**: Streaming support for files of any size

## License

This package wraps the **FLAC** library, which is licensed under **BSD-style licenses**.

## See Also

- [Common Decoders Documentation](../README.md) - API interface, usage patterns, RingBuffer integration
- [MP3 Decoder](../mp3/README.md) - Lossy compressed audio
- [WAV Decoder](../wav/README.md) - Uncompressed PCM audio
- [go-flac Library](https://github.com/drgolem/go-flac)
