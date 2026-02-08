# FramesPlayer - Frame-Based Audio Player

Audio player demonstrating callback-based playback using `AudioFrameRingBuffer` for frame-based buffering with format metadata.

## Overview

This example shows how to use `AudioFrameRingBuffer` for audio playback, where each buffered item is an `AudioFrame` object containing:
- Audio format metadata (sample rate, channels, bits per sample)
- Sample count
- Raw audio data

**Comparison with play_callback:**

| Feature | framesplayer | play_callback |
|---------|-------------|---------------|
| **Buffer Type** | AudioFrameRingBuffer | RingBuffer (bytes) |
| **Data Unit** | AudioFrame objects | Raw bytes |
| **Metadata** | ✅ Format per frame | ❌ Global only |
| **Use Case** | Variable formats | Fixed format |
| **Complexity** | Higher | Lower |
| **Overhead** | Frame objects | Minimal |

## Features

- ✅ **Frame-based buffering** - Each frame includes format metadata
- ✅ **Partial frame consumption** - Handles frame boundaries in callback
- ✅ **Lock-free SPSC** - Producer/consumer with atomic operations
- ✅ **Low latency** - Direct PortAudio callback mode
- ✅ **Multi-format** - Supports MP3, FLAC, WAV
- ✅ **Configurable** - Adjust buffer capacity and frame sizes

## Building

```bash
cd pkg/audioplayer/examples/framesplayer
go build -o framesplayer main.go
```

Or from project root:

```bash
go build -o framesplayer ./pkg/audioplayer/examples/framesplayer/
```

## Usage

```bash
./framesplayer [options] <audio_file>
```

### Options

- `-device N` - Audio output device index (default: 1)
- `-capacity N` - Ring buffer capacity in frames (default: 256)
- `-paframes N` - PortAudio frames per buffer (default: 512)
- `-samples N` - Samples per AudioFrame (default: 4096)
- `-v` - Verbose output with buffer statistics

### Examples

**Basic playback:**
```bash
./framesplayer music.mp3
```

**Select audio device:**
```bash
./framesplayer -device 0 music.flac
```

**Verbose mode with buffer monitoring:**
```bash
./framesplayer -v music.wav
```

**Custom buffer configuration:**
```bash
# Larger buffer, smaller frames
./framesplayer -capacity 512 -samples 2048 music.mp3

# Smaller buffer, larger frames
./framesplayer -capacity 128 -samples 8192 music.flac
```

## Architecture

### Producer-Consumer Pattern

```
┌─────────┐     ┌──────────────────────┐     ┌──────────┐
│ Decoder │────▶│ AudioFrameRingBuffer │────▶│ Callback │
│ (Thread)│     │   (Lock-free SPSC)   │     │ (RT)     │
└─────────┘     └──────────────────────┘     └──────────┘
     │                                              │
     │                                              │
     ▼                                              ▼
AudioFrame                                   Extract bytes
+ Format                                     → PortAudio
+ SamplesCount
+ Audio []byte
```

### Producer (Decoder Thread)

1. Decode audio samples from file
2. Create `AudioFrame` with:
   - Format metadata (rate, channels, depth)
   - Sample count
   - Audio bytes
3. Write frame to `AudioFrameRingBuffer`
4. Repeat until end of file

### Consumer (Audio Callback - Real-Time)

1. Calculate bytes needed for PortAudio buffer
2. Read `AudioFrame` from ring buffer
3. Extract audio bytes from frame
4. Handle partial frame consumption:
   - Track current frame and offset
   - Copy bytes to output buffer
   - Move to next frame when exhausted
5. Fill remainder with silence if needed

## Frame-Based Buffering

### Why Frame-Based?

**Advantages:**
- ✅ Format metadata travels with audio data
- ✅ Can handle format changes mid-stream
- ✅ Easier debugging (inspect frame metadata)
- ✅ Higher-level abstraction

**Trade-offs:**
- ❌ Slightly more overhead (frame objects)
- ❌ More complex callback logic
- ❌ Potential for partial frame handling

### Frame Size Considerations

**Larger frames (8192+ samples):**
- ✅ Fewer frame objects
- ✅ Less overhead per byte
- ❌ Higher latency
- ❌ More memory per frame

**Smaller frames (1024-2048 samples):**
- ✅ Lower latency
- ✅ More granular buffering
- ❌ More frame objects
- ❌ More overhead

**Recommended:** 4096 samples (default)
- Good balance for most use cases
- ~93ms latency at 44.1kHz
- ~85ms latency at 48kHz

## Buffer Capacity

The `-capacity` flag sets the number of frames the ring buffer can hold.

**Calculation:**
```
Total buffer time = (capacity × samples_per_frame) / sample_rate

Example (default):
- Capacity: 256 frames
- Samples per frame: 4096
- Sample rate: 44100 Hz
- Buffer time: (256 × 4096) / 44100 = 23.8 seconds
```

**Guidelines:**
- Streaming: 128-256 frames (sufficient)
- Local files: 256-512 frames (smooth playback)
- Slow disks: 512-1024 frames (prevent underruns)

## Callback Mechanics

### Partial Frame Consumption

The callback handles frames that don't align with PortAudio buffer boundaries:

```go
// State maintained between callbacks
currentFrame *audioframe.AudioFrame
frameOffset  int  // bytes consumed from currentFrame

// In callback:
for bytesWritten < bytesNeeded {
    if currentFrame == nil {
        // Get next frame
        frames, _ := ringbuf.Read(1)
        currentFrame = &frames[0]
        frameOffset = 0
    }

    // Copy bytes from current frame
    bytesToCopy := min(remainingInFrame, remainingInOutput)
    copy(output[bytesWritten:], currentFrame.Audio[frameOffset:])

    bytesWritten += bytesToCopy
    frameOffset += bytesToCopy

    // Move to next frame if exhausted
    if frameOffset >= len(currentFrame.Audio) {
        currentFrame = nil
    }
}
```

### Real-Time Safety

The callback runs in PortAudio's real-time audio thread:
- ✅ **No allocations** - reuses frame objects
- ✅ **No blocking** - lock-free ring buffer
- ✅ **Fast path** - simple copy operations
- ✅ **Graceful underrun** - fills with silence

## Performance

### Typical Metrics

**44.1kHz stereo 16-bit, 4096 samples/frame:**
- Frame size: 16,384 bytes (4096 × 2 × 2)
- Frames per second: ~10.8
- Buffer capacity (256 frames): ~23.8 seconds

**Buffer overhead:**
- AudioFrame struct: ~32 bytes
- Format metadata: 6 bytes
- Total per frame: ~38 bytes + audio data

**Throughput:**
- Write: ~12.6 ns/op (0 allocs)
- Read: ~70.5 ns/op (1 alloc per Read call)

## Troubleshooting

### Audio Stuttering/Dropouts

**Symptoms:** Intermittent silence or glitches

**Solutions:**
1. Increase buffer capacity: `-capacity 512`
2. Increase frame size: `-samples 8192`
3. Check CPU usage (high load can cause issues)
4. Try different audio device: `-device 0`

### High Latency

**Symptoms:** Delayed audio response

**Solutions:**
1. Decrease frame size: `-samples 2048`
2. Decrease buffer capacity: `-capacity 128`
3. Decrease PortAudio buffer: `-paframes 256`

### Buffer Monitoring

Use verbose mode to see buffer status:

```bash
./framesplayer -v music.mp3
```

Output includes:
```
level=DEBUG msg="Buffer status" available_frames=180 capacity=256 fill_percent=70.3%
```

**Healthy buffer:**
- Fill: 30-80% most of time
- Oscillates as producer/consumer work

**Warning signs:**
- Fill: 0-10% (underrun risk)
- Fill: 90-100% (producer stalling)

## Comparison with play_callback

| Aspect | framesplayer | play_callback |
|--------|-------------|---------------|
| **Buffer Unit** | AudioFrame objects | Raw bytes |
| **Metadata** | Per frame | Global |
| **Callback Complexity** | Higher (frame boundaries) | Lower (direct copy) |
| **Memory Overhead** | ~38 bytes/frame | None |
| **Flexibility** | Format can change | Fixed format |
| **Performance** | Slightly slower | Fastest |
| **Use Case** | Complex pipelines | Simple streaming |

**When to use framesplayer:**
- Need format metadata per frame
- Building complex audio pipeline
- Processing variable-format streams
- Debugging audio data flow

**When to use play_callback:**
- Maximum performance needed
- Simple fixed-format streaming
- Low-level audio control
- Minimal overhead required

## Example Output

```
level=INFO msg="Initializing PortAudio"
level=INFO msg="PortAudio initialized" version="PortAudio V19.7.0-devel, revision unknown"
level=INFO msg="Configuration" device_index=1 frame_capacity=256 pa_frames_per_buffer=512 samples_per_audioframe=4096
level=INFO msg="Opening file" path="music.mp3"
level=INFO msg="Audio file opened" sample_rate=44100 channels=2 bits_per_sample=16 samples_per_frame=4096
level=INFO msg="Playback started (frame-based callback mode)"
level=INFO msg="Producer started" samples_per_frame=4096 buffer_bytes=16384
level=INFO msg="Producer finished" error=<nil> samples_read=0 total_frames=2643
level=INFO msg="Playback completed"
level=INFO msg="Playback stopped"
level=INFO msg="Exiting"
```

## See Also

- [AudioFrameRingBuffer](../../../audioframeringbuffer/README.md) - Frame-based ring buffer
- [AudioFrame](../../../audioframe/README.md) - Binary frame format
- [play_callback](../play_callback/) - Byte-based callback player
- [play](../play/) - Blocking I/O player
