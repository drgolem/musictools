# MP3 Decoder

**📖 For common API documentation, usage patterns, and integration examples, see [Common Decoders Documentation](../README.md)**

A Go package for decoding MP3 files using the [go-mpg123](https://github.com/drgolem/go-mpg123) library, which wraps the mpg123 decoder.

## MP3-Specific Features

- Decode MP3 files to raw PCM audio data
- Support for various MP3 encoding formats (MPEG-1, MPEG-2, MPEG-2.5)
- SIMD-optimized decoding via mpg123 library
- Fixed 16-bit output from lossy compressed source

## Installation

This package requires the **mpg123** library to be installed on your system.

```bash
go get github.com/drgolem/go-mpg123
```

### macOS
```bash
brew install mpg123
```

### Ubuntu/Debian
```bash
sudo apt-get install libmpg123-dev
```

### Fedora/RHEL
```bash
sudo dnf install mpg123-devel
```

## MP3 Encoding Formats

MP3 decoders output 16-bit signed PCM regardless of the source encoding. The `GetFormat()` method returns an encoding code from mpg123:

| Sample Rate | Channels | Description |
|-------------|----------|-------------|
| 44100 Hz | 2 | CD quality stereo (most common) |
| 48000 Hz | 2 | Professional audio stereo |
| 44100 Hz | 1 | CD quality mono |
| 48000 Hz | 1 | Professional audio mono |
| 22050 Hz | 2 | Half CD quality stereo |
| 32000 Hz | 2 | DAB audio |

**Note:** MP3 always decodes to 16-bit PCM. Unlike FLAC or WAV, higher bit depths are not supported.

## MP3-Specific API Methods

### `func (d *Decoder) Encoding() int`

Returns the mpg123 encoding format code. This is MP3-specific and differs from the standard `bitsPerSample` in the `GetFormat()` method.

```go
rate, channels, encoding := decoder.GetFormat()
fmt.Printf("Encoding code: %d\n", encoding)
```

## Example: Decode MP3 to Raw PCM

The package includes a command-line tool to decode MP3 files:

```bash
# Build the decoder example
cd pkg/decoders/mp3/examples/decode
go build -o decode main.go

# Decode an MP3 file to files
./decode music.mp3

# Pipe directly to ffplay (no intermediate files!)
./decode music.mp3 --pipe | ffplay -f s16le -ar 44100 -ch_layout stereo -

# Stream and convert to WAV
./decode music.mp3 --pipe | ffmpeg -f s16le -ar 44100 -ch_layout stereo -i - output.wav
```

### Output Files

When saving to files (file mode), the tool creates:
- `output.raw` - Raw PCM audio data (16-bit signed little-endian)
- `output.meta` - JSON metadata with format information

**Metadata format:**
```json
{
  "sample_rate": 44100,
  "channels": 2,
  "encoding": 208,
  "source_file": "music.mp3",
  "raw_file": "output.raw"
}
```

## Performance

The decoder uses the highly optimized **mpg123** library, which provides:
- **Fast decoding**: 100x+ real-time on modern systems
- **SIMD optimizations**: Platform-specific optimizations (SSE, AVX, NEON)
- **Low CPU usage**: Efficient decoding algorithms
- **Minimal memory allocations**: Zero-allocation after initialization

## License

This package wraps the **mpg123** library, which is licensed under **LGPL 2.1**.

## See Also

- [Common Decoders Documentation](../README.md) - API interface, usage patterns, RingBuffer integration
- [FLAC Decoder](../flac/README.md) - Lossless audio decoding
- [WAV Decoder](../wav/README.md) - Uncompressed PCM decoding
- [go-mpg123 Library](https://github.com/drgolem/go-mpg123)
