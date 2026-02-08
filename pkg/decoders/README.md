# Audio Decoders

A collection of high-performance audio decoders for Go, supporting MP3, FLAC, and WAV formats. All decoders provide a consistent `AudioDecoder` interface for seamless integration with the ringbuffer-based audio streaming system.

## Available Decoders

- **[factory.go](./factory.go)** - Decoder factory for automatic format detection
- **[MP3 Decoder](./mp3/README.md)** - Decode MP3 files using mpg123
- **[FLAC Decoder](./flac/README.md)** - Decode FLAC files with lossless quality
- **[WAV Decoder](./wav/README.md)** - Decode uncompressed PCM WAV files

## Common API Interface

All decoders implement the `types.AudioDecoder` interface defined in `pkg/types`:

```go
type AudioDecoder interface {
    Open(fileName string) error
    Close() error
    GetFormat() (rate, channels, bitsPerSample int)
    DecodeSamples(samples int, audio []byte) (int, error)
}
```

This allows you to write format-agnostic code that works with any supported audio format.

## Recommended Usage: Decoder Factory

**Always use the decoder factory** for automatic format detection based on file extension:

```go
import "musictools/pkg/decoders"

// Factory automatically detects format and creates the right decoder
decoder, err := decoders.NewDecoder("music.mp3")
if err != nil {
    log.Fatal(err)
}
defer decoder.Close()

// Decoder is ready to use - format detected automatically
rate, channels, bitsPerSample := decoder.GetFormat()
```

**Supported extensions**: `.mp3`, `.flac`, `.fla`, `.wav`

### Benefits of Using the Factory

- ✅ **Single source of truth** - One place for format detection logic
- ✅ **Automatic format detection** - No need to know file format in advance
- ✅ **Error handling** - Clear errors for unsupported formats
- ✅ **Maintainable** - Adding new formats requires updating only the factory

## Direct Decoder Creation (Advanced)

For advanced use cases, you can create decoders directly:

```go
import "musictools/pkg/decoders/mp3"  // or flac, wav

// Create decoder directly
decoder := mp3.NewDecoder()
defer decoder.Close()

// Open audio file
err := decoder.Open("music.mp3")
if err != nil {
    log.Fatal(err)
}
```

**Note**: Direct creation is discouraged - prefer the factory for maintainability.

### Getting Audio Format Information

```go
// Get format information
rate, channels, bitsPerSample := decoder.GetFormat()
fmt.Printf("Format: %d Hz, %d channels, %d-bit\n", rate, channels, bitsPerSample)
```

### Decoding Audio Samples

```go
// Prepare buffer for decoding
bufferSamples := 1024
bytesPerSample := bitsPerSample / 8
bufferSize := bufferSamples * channels * bytesPerSample
buffer := make([]byte, bufferSize)

// Decode samples in a loop
for {
    samplesRead, err := decoder.DecodeSamples(bufferSamples, buffer)
    if samplesRead == 0 || err != nil {
        break
    }

    bytesRead := samplesRead * channels * bytesPerSample
    // Process buffer[:bytesRead] - contains raw PCM audio data
}
```

## Integration with RingBuffer

All decoders work seamlessly with the lock-free ringbuffer for real-time audio streaming:

```go
import (
    "musictools/pkg/decoders"
    "musictools/pkg/ringbuffer"
    "time"
)

// Create decoder using factory
decoder, err := decoders.NewDecoder("music.mp3")
if err != nil {
    log.Fatal(err)
}
defer decoder.Close()

rb := ringbuffer.New(64 * 1024) // 64KB buffer

// Producer: decode and write to ringbuffer
go func() {
    buffer := make([]byte, 4096)
    for {
        samplesRead, err := decoder.DecodeSamples(1024, buffer)
        if samplesRead == 0 || err != nil {
            break
        }

        bytesToWrite := samplesRead * 2 * 2 // stereo 16-bit

        // Wait for space if needed
        for rb.AvailableWrite() < uint64(bytesToWrite) {
            time.Sleep(time.Millisecond)
        }

        rb.Write(buffer[:bytesToWrite])
    }
}()

// Consumer: read from ringbuffer and play
go func() {
    chunk := make([]byte, 4096)
    for {
        n, err := rb.Read(chunk)
        if err != nil {
            continue
        }
        // Send chunk[:n] to audio output device
    }
}()
```

## Common API Methods

### `NewDecoder() *Decoder`

Creates a new decoder instance for the specified format.

### `Open(fileName string) error`

Opens an audio file for decoding. Must be called before `DecodeSamples()`.

**Returns:** Error if file not found, invalid format, or unsupported encoding.

### `Close() error`

Closes the decoder and releases resources. Safe to call multiple times.

### `GetFormat() (rate, channels, bitsPerSample int)`

Returns the audio format information:
- `rate`: Sample rate in Hz (e.g., 44100, 48000, 96000)
- `channels`: Number of channels (1=mono, 2=stereo, etc.)
- `bitsPerSample`: Bit depth (8, 16, 24, or 32)

### `DecodeSamples(samples int, audio []byte) (int, error)`

Decodes audio samples into the provided buffer.

**Parameters:**
- `samples`: Number of samples to decode (not bytes!)
- `audio`: Buffer to write decoded audio data

**Returns:**
- Number of samples actually decoded
- Error if decoding failed

**Important:** Buffer must be large enough to hold: `samples * channels * (bitsPerSample/8)` bytes

**Example:**
```go
// Decode 1024 samples from stereo 16-bit audio
rate, channels, bitsPerSample := decoder.GetFormat()
samples := 1024
bufferSize := samples * channels * (bitsPerSample / 8)  // 4096 bytes for 16-bit stereo
buffer := make([]byte, bufferSize)

samplesRead, err := decoder.DecodeSamples(samples, buffer)
if err != nil {
    log.Fatal(err)
}

bytesRead := samplesRead * channels * (bitsPerSample / 8)
processAudio(buffer[:bytesRead])
```

## Playing Raw PCM Data

### Using ffplay

Play decoded raw PCM data directly:

```bash
# Basic playback (16-bit stereo at 44.1kHz)
ffplay -f s16le -ar 44100 -ch_layout stereo output.raw

# Mono audio
ffplay -f s16le -ar 48000 -ch_layout mono output.raw

# High-quality 24-bit audio
ffplay -f s24le -ar 96000 -ch_layout stereo output.raw
```

**Format flags:**
- `-f FORMAT` - PCM format: `s16le` (16-bit), `s24le` (24-bit), `s32le` (32-bit)
- `-ar RATE` - Audio sample rate in Hz
- `-ch_layout LAYOUT` - Channel layout (stereo, mono, 5.1, etc.)

### Converting to WAV

Convert raw PCM to WAV format:

```bash
# Create WAV file from raw PCM
ffmpeg -f s16le -ar 44100 -ch_layout stereo -i output.raw output.wav

# With metadata
ffmpeg -f s16le -ar 44100 -ch_layout stereo -i output.raw \
  -metadata title="Song Title" \
  -metadata artist="Artist Name" \
  output.wav

# High-quality 24-bit conversion
ffmpeg -f s24le -ar 96000 -ch_layout stereo -i output.raw -sample_fmt s24 output.wav
```

## Error Handling

Common errors across all decoders:

- **File not found or cannot be opened** - Check file path and permissions
- **Invalid file format** - File may be corrupted or wrong format
- **Library not installed** - Install required system library (see format-specific docs)
- **Decoder not initialized** - Call `Open()` before `DecodeSamples()`
- **Unsupported format** - File uses unsupported compression or bit depth

**Example error handling:**
```go
decoder := mp3.NewDecoder()
defer decoder.Close()

err := decoder.Open("music.mp3")
if err != nil {
    if strings.Contains(err.Error(), "no such file") {
        log.Fatal("File not found:", err)
    } else if strings.Contains(err.Error(), "invalid") {
        log.Fatal("Invalid audio file:", err)
    } else {
        log.Fatal("Decoder error:", err)
    }
}
```

## Performance Characteristics

All decoders are highly optimized:

- **Fast decoding** - 100x+ real-time on modern systems
- **Low CPU usage** - Optimized C libraries with SIMD support
- **Minimal memory allocations** - Zero-allocation after initialization
- **Streaming support** - Process files of any size without loading into memory

### Memory Usage

Decoders use minimal memory:
- Fixed overhead for decoder state (~few KB)
- Single buffer allocation (user-controlled size)
- No internal buffering of entire files

### CPU Usage

All decoders leverage optimized C libraries or direct PCM access:
- **MP3**: mpg123 library with SIMD optimizations
- **FLAC**: FLAC reference library with streaming support
- **WAV**: Direct PCM read (no decompression needed, fastest)

## Choosing a Decoder

| Factor | MP3 | FLAC | WAV |
|--------|-----|------|-----|
| **File Size** | Small (lossy) | Medium (lossless) | Large (uncompressed) |
| **Quality** | Good | Excellent | Original |
| **Speed** | Medium | Medium | Fastest |
| **Use Case** | Streaming, distribution | Archival, hi-fi | Editing, master storage |
| **Bit Depth** | 16-bit fixed | 16/24/32-bit | 8/16/24/32-bit |
| **Sample Rates** | Up to 48kHz typical | Up to 192kHz+ | Any |
| **CPU Usage** | Low | Low-Medium | Minimal |

## Format-Specific Documentation

For format-specific details, installation instructions, and advanced features:

- **[MP3 Decoder →](./mp3/README.md)** - Encoding formats, mpg123 installation
- **[FLAC Decoder →](./flac/README.md)** - Bit depth support, lossless codec details
- **[WAV Decoder →](./wav/README.md)** - Sample layouts, format support matrix

## License

Each decoder package wraps a native library or uses Go libraries with their own licenses:
- **MP3**: mpg123 (LGPL 2.1)
- **FLAC**: FLAC reference library (BSD-style)
- **WAV**: go-wav library (MIT-style)

See individual decoder READMEs for detailed license information.
