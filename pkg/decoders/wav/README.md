# WAV Decoder

**📖 For common API documentation, usage patterns, and integration examples, see [Common Decoders Documentation](../README.md)**

WAV audio decoder using [github.com/youpy/go-wav](https://github.com/youpy/go-wav).

## WAV-Specific Features

- **PCM Support**: Uncompressed PCM audio (fastest decoding)
- **Multiple Bit Depths**: 8-bit, 16-bit, 24-bit, 32-bit
- **Multi-Channel**: Mono, stereo, and multi-channel audio
- **Direct PCM Access**: No decompression overhead
- **Editing-Friendly**: Industry standard for audio editing

## Supported Formats

| Format | Bit Depth | Channels | Status |
|--------|-----------|----------|--------|
| **PCM** | 8-bit | Any | ✅ Supported |
| **PCM** | 16-bit | Any | ✅ Supported |
| **PCM** | 24-bit | Any | ✅ Supported |
| **PCM** | 32-bit | Any | ✅ Supported |
| **ADPCM** | Any | Any | ❌ Not supported |
| **μ-law** | Any | Any | ❌ Not supported |
| **A-law** | Any | Any | ❌ Not supported |

**Note:** This decoder only supports **PCM (uncompressed)** WAV files. Other WAV formats require conversion.

## WAV Sample Layouts

WAV files use **little-endian** PCM format with interleaved channels.

### 16-bit Stereo Layout

```
Byte:  [0][1] [2][3] [4][5] [6][7] [8][9] ...
Data:  [L0  ] [R0  ] [L1  ] [R1  ] [L2  ] ...
       Sample0        Sample1        Sample2
```

Each sample consists of:
- **Left channel**: 2 bytes (low byte, high byte)
- **Right channel**: 2 bytes (low byte, high byte)

### 24-bit Stereo Layout

```
Byte:  [0][1][2] [3][4][5] [6][7][8] [9][10][11] ...
Data:  [L0    ] [R0    ] [L1     ] [R1      ] ...
       Sample0           Sample1
```

Each sample consists of:
- **Left channel**: 3 bytes (LSB, middle, MSB)
- **Right channel**: 3 bytes (LSB, middle, MSB)

### Mono vs Stereo

**Mono (1 channel):**
```
[Sample0] [Sample1] [Sample2] [Sample3] ...
```

**Stereo (2 channels):**
```
[L0] [R0] [L1] [R1] [L2] [R2] ...
```

## Running the Example

```bash
# Build the decoder example
cd pkg/decoders/wav/examples/decode
go build -o decode main.go

# Decode a WAV file
./decode audio.wav

# Example output:
# Opening: audio.wav
# Sample Rate: 44100 Hz
# Channels: 2
# Bits Per Sample: 16
#
# Decoding 1024 samples at a time...
# Total samples decoded: 132300
# Duration: 3.00 seconds
# Total audio data: 529200 bytes (0.50 MB)
```

## WAV vs Other Formats

| Feature | WAV | MP3 | FLAC |
|---------|-----|-----|------|
| **Compression** | None | Lossy | Lossless |
| **File Size** | Largest | Smallest | Medium |
| **Decode Speed** | **Fastest** | Medium | Medium |
| **Quality** | Original | Lossy | Original |
| **Use Case** | Studio, editing | Streaming | Archival |
| **CPU Usage** | **Minimal** | Low | Low-Medium |

**When to use WAV:**
- Audio editing and production
- Master file storage
- When quality is paramount
- Short audio clips where file size is not critical

## WAV-Specific Troubleshooting

### "Unsupported WAV format"

WAV supports many formats (PCM, ADPCM, μ-law, etc.). This decoder only supports **PCM** (uncompressed).

**Check format with ffprobe:**
```bash
ffprobe audio.wav
# Look for: "Stream #0:0: Audio: pcm_s16le" (PCM format)
```

**Convert to PCM if needed:**
```bash
# Convert any WAV to PCM 16-bit
ffmpeg -i input.wav -acodec pcm_s16le -ar 44100 output.wav

# Convert to PCM 24-bit
ffmpeg -i input.wav -acodec pcm_s24le -ar 48000 output.wav
```

### Buffer Size Calculation

A common mistake is using incorrect buffer sizes:

```go
// ❌ WRONG: Buffer too small for stereo 16-bit
samples := 1024
buffer := make([]byte, 1024)  // This is too small!
decoder.DecodeSamples(samples, buffer)

// ✅ CORRECT: Calculate proper buffer size
rate, channels, bitsPerSample := decoder.GetFormat()
bufferSize := samples * channels * (bitsPerSample / 8)
buffer := make([]byte, bufferSize)  // 4096 bytes for stereo 16-bit
decoder.DecodeSamples(samples, buffer)
```

**Buffer size formula:**
```
bufferSize = samples × channels × (bitsPerSample ÷ 8)
```

**Examples:**
- 1024 samples, stereo (2), 16-bit: 1024 × 2 × 2 = **4096 bytes**
- 1024 samples, stereo (2), 24-bit: 1024 × 2 × 3 = **6144 bytes**
- 1024 samples, mono (1), 16-bit: 1024 × 1 × 2 = **2048 bytes**

### Performance Optimization

```go
// Use larger buffer sizes for better performance
samples := 4096  // Instead of 1024 - reduces overhead

// Pre-allocate buffer once
buffer := make([]byte, samples * channels * bytesPerSample)

// Reuse buffer in loop (zero allocations)
for {
    samplesRead, err := decoder.DecodeSamples(samples, buffer)
    if err != nil || samplesRead == 0 {
        break
    }
    bytesRead := samplesRead * channels * bytesPerSample
    processAudio(buffer[:bytesRead])
}
```

## Example: Extract Channels

Extract left channel from stereo WAV:

```go
rate, channels, bps := decoder.GetFormat()
if channels != 2 {
    panic("Not stereo")
}

bytesPerSample := bps / 8
buffer := make([]byte, 1024*channels*bytesPerSample)
leftChannel := make([]byte, 1024*bytesPerSample)

for {
    samplesRead, _ := decoder.DecodeSamples(1024, buffer)
    if samplesRead == 0 {
        break
    }

    // Extract left channel
    for i := 0; i < samplesRead; i++ {
        srcOffset := i * channels * bytesPerSample
        dstOffset := i * bytesPerSample
        copy(leftChannel[dstOffset:], buffer[srcOffset:srcOffset+bytesPerSample])
    }

    processLeftChannel(leftChannel[:samplesRead*bytesPerSample])
}
```

## Performance Characteristics

- **Decoding Speed**: ~1000x real-time (44.1kHz audio) - fastest of all formats
- **Memory Usage**: Minimal (only buffer allocation)
- **CPU Usage**: Very low (no decompression needed)

WAV is the fastest decoder because it directly reads PCM data without any decompression.

## See Also

- [Common Decoders Documentation](../README.md) - API interface, usage patterns, RingBuffer integration
- [MP3 Decoder](../mp3/README.md) - Lossy compressed audio
- [FLAC Decoder](../flac/README.md) - Lossless compressed audio
- [go-wav Library](https://github.com/youpy/go-wav)
