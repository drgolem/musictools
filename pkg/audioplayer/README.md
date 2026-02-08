# Audio Player with Producer/Consumer Pattern

A real-time audio player that demonstrates the producer/consumer pattern using:
- **Lock-free ringbuffer** for zero-copy audio data passing
- **Audio decoders** (MP3, FLAC) as the producer
- **PortAudio** for audio output as the consumer

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│   Decoder   │ write   │  Ringbuffer  │  read   │  PortAudio   │
│  (Producer) ├────────>│  (Lock-Free) ├────────>│  (Consumer)  │
│  Goroutine  │         │     SPSC     │         │   Callback   │
└─────────────┘         └──────────────┘         └──────────────┘
```

### Components

1. **Producer (Goroutine)**
   - Decodes audio from file (MP3 or FLAC)
   - Writes decoded PCM data to ringbuffer
   - Blocks when ringbuffer is full (backpressure)

2. **Ringbuffer (Lock-Free SPSC)**
   - Single-Producer Single-Consumer lock-free buffer
   - Zero-copy reads using `ReadSlices()`
   - Efficient memory usage (power-of-2 size)
   - Thread-safe without locks

3. **Consumer (PortAudio Callback)**
   - Called by audio hardware when it needs data
   - Reads from ringbuffer using zero-copy
   - Must be real-time safe (no allocations, no locks)
   - Outputs silence on buffer underrun

## Features

- **Zero-Copy Data Transfer**: Uses `ReadSlices()` for zero-copy reads from ringbuffer
- **Real-Time Safe Consumer**: Audio callback never blocks or allocates
- **Automatic Format Detection**: Supports MP3 and FLAC files
- **Graceful Shutdown**: Clean shutdown with signal handling
- **Buffer Monitoring**: Real-time buffer status reporting

## Installation

```bash
# Install PortAudio library
# macOS
brew install portaudio

# Ubuntu/Debian
sudo apt-get install portaudio19-dev

# Fedora/RHEL
sudo dnf install portaudio-devel
```

## Usage

### Basic Example

```bash
# Build and run
cd pkg/audioplayer/examples/play
go build -o play main.go

# Play an MP3 file
./play music.mp3

# Play a FLAC file
./play music.flac
```

### Programmatic Usage

```go
package main

import (
    "learnRingbuffer/pkg/audioplayer"
    "github.com/drgolem/go-portaudio/portaudio"
)

func main() {
    // Initialize PortAudio
    portaudio.Initialize()
    defer portaudio.Terminate()

    // Create player with custom config
    config := audioplayer.Config{
        BufferSize:      512 * 1024, // 512KB ringbuffer
        FramesPerBuffer: 1024,       // 1024 frames per callback
    }
    player := audioplayer.NewPlayer(config)

    // Open and play file
    if err := player.OpenFile("music.mp3"); err != nil {
        panic(err)
    }

    if err := player.Play(); err != nil {
        panic(err)
    }

    // Wait for playback to complete
    player.Wait()

    // Or stop manually
    // player.Stop()
}
```

## Configuration

### Config Options

```go
type Config struct {
    BufferSize      uint64 // Ringbuffer size in bytes
    FramesPerBuffer int    // PortAudio callback buffer size
}
```

**BufferSize**: Larger buffer = more latency, less chance of underruns
- Small (64KB): Low latency, higher CPU usage
- Medium (256KB): Balanced (default)
- Large (1MB): High latency, more resilient

**FramesPerBuffer**: Audio callback granularity
- Small (256): Lower latency, more frequent callbacks
- Medium (512): Balanced (default)
- Large (2048): Higher latency, lower CPU usage

## How It Works

### Producer (Decoder Thread)

```go
// 1. Decode audio samples from file
samplesRead, _ := decoder.DecodeSamples(4096, buffer)

// 2. Calculate bytes to write
bytesToWrite := samplesRead * channels * bytesPerSample

// 3. Write to ringbuffer (blocks if full)
ringbuf.Write(buffer[:bytesToWrite])
```

### Consumer (Audio Thread)

```go
// 1. Get zero-copy slices from ringbuffer
first, second, available := ringbuf.ReadSlices()

// 2. Copy data to output buffer
copy(outputBuffer, first)
if second != nil {
    copy(outputBuffer[len(first):], second)
}

// 3. Advance ringbuffer read position
ringbuf.Consume(bytesRead)
```

## Performance

- **Zero Allocations**: Audio callback has zero allocations
- **Lock-Free**: No mutex contention between producer/consumer
- **Real-Time Safe**: Consumer thread never blocks
- **Efficient**: Uses atomic operations for synchronization

## Buffer Status

The player provides real-time buffer monitoring:

```go
available, size := player.GetBufferStatus()
percentage := float64(available) / float64(size) * 100
```

Expected behavior:
- **75-100% full**: Producer is faster than consumer (good)
- **25-75% full**: Balanced
- **0-25% full**: Risk of underruns
- **0% (underrun)**: Consumer outputs silence until buffer fills

## Supported Formats

| Format | Extensions | Bit Depths |
|--------|-----------|------------|
| MP3    | .mp3      | 16-bit     |
| FLAC   | .flac     | 16, 24, 32-bit |

## Error Handling

The player handles:
- Buffer underruns (outputs silence)
- End of file (drains buffer, then stops)
- Signal interrupts (SIGINT, SIGTERM)
- Stream errors (logs and attempts recovery)

## Examples

### Play with Buffer Monitoring

```bash
./play music.flac
```

Output:
```
time=2025-01-15T10:00:00.000Z level=INFO msg="Audio file opened" sample_rate=44100 channels=2 bits_per_sample=16
time=2025-01-15T10:00:00.100Z level=INFO msg="Producer started"
time=2025-01-15T10:00:00.100Z level=INFO msg="Playback started"
time=2025-01-15T10:00:01.000Z level=INFO msg="Buffer status" available_bytes=245760 buffer_size=262144 fill_percentage=93.8%
time=2025-01-15T10:00:02.000Z level=INFO msg="Buffer status" available_bytes=253952 buffer_size=262144 fill_percentage=96.9%
```

### Graceful Shutdown (Ctrl+C)

```bash
./play music.mp3
^C
time=2025-01-15T10:00:05.000Z level=INFO msg="Interrupt received, stopping playback"
time=2025-01-15T10:00:05.001Z level=INFO msg="Producer stopped"
time=2025-01-15T10:00:05.002Z level=INFO msg="Playback stopped"
```

## Troubleshooting

### Buffer Underruns

If you see buffer underruns (audio glitches):
1. Increase `BufferSize` (more buffering)
2. Increase `FramesPerBuffer` (less frequent callbacks)
3. Close other audio applications
4. Check system load

### High CPU Usage

If CPU usage is high:
1. Increase `FramesPerBuffer` (fewer callbacks)
2. Use lower sample rate files
3. Check for excessive logging

### Latency Issues

If latency is too high:
1. Decrease `BufferSize`
2. Decrease `FramesPerBuffer`
3. Trade-off: Lower latency = higher chance of underruns

## Design Principles

1. **Zero-Copy Where Possible**: Audio callback uses `ReadSlices()` for zero-copy reads
2. **Real-Time Safety**: Audio callback never allocates or blocks
3. **Separation of Concerns**: Producer, consumer, and buffer are independent
4. **Graceful Degradation**: Outputs silence on underrun instead of crashing
5. **Observable**: Provides buffer status monitoring

## See Also

- [MP3 Decoder](../decoders/mp3/README.md)
- [FLAC Decoder](../decoders/flac/README.md)
- [Ringbuffer](../ringbuffer/README.md)
- [PortAudio Documentation](http://www.portaudio.com/docs/v19-doxydocs/)
