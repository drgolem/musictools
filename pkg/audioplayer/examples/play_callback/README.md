# PortAudio Callback Mode Example

This example demonstrates audio playback using PortAudio's **callback mode** with a ringbuffer for buffering.

## Callback Mode vs Blocking I/O

### Blocking I/O Mode (examples/play/)
- **How it works**: Application calls `stream.Write()` to push audio data
- **Control**: Application controls when data is sent
- **Thread model**: Consumer goroutine actively writes to PortAudio
- **Latency**: Higher latency (uses `DefaultHighOutputLatency`)
- **Best for**: Simple applications, where control flow is important

### Callback Mode (this example)
- **How it works**: PortAudio calls your function to request audio data
- **Control**: PortAudio controls when data is needed (pull model)
- **Thread model**: PortAudio's real-time thread calls your callback
- **Latency**: Lower latency (uses `DefaultLowOutputLatency`)
- **Best for**: Professional audio, real-time processing, low-latency applications

## Architecture

```
File → Decoder → Producer Goroutine → RingBuffer ← Callback ← PortAudio → Output
                 (background)                      (RT thread)
```

1. **Producer Goroutine**: Runs in background, decodes audio and fills ringbuffer
2. **Callback Function**: Runs in PortAudio's real-time thread, pulls from ringbuffer
3. **RingBuffer**: Lock-free SPSC buffer connecting producer and callback

## Key Features

### Zero-Copy Audio Transfer
The callback uses `ringbuf.ReadSlices()` for zero-copy access:
```go
first, second, available := cp.ringbuf.ReadSlices()
copy(output, first)
if second != nil {
    copy(output[len(first):], second)
}
cp.ringbuf.Consume(bytesRead)
```

### Real-Time Safe Callback
The callback follows real-time programming rules:
- ✅ No memory allocation
- ✅ No blocking operations
- ✅ No file I/O or logging
- ✅ Predictable execution time
- ✅ Lock-free data structures (ringbuffer)

### Graceful Underrun Handling
If the ringbuffer is empty, the callback outputs silence:
```go
if available == 0 {
    // Output silence instead of stopping
    for i := range output[:bytesNeeded] {
        output[i] = 0
    }
    return portaudio.Continue
}
```

### Automatic Completion Detection
When the producer finishes and buffer drains:
```go
if cp.producerDone.Load() && cp.ringbuf.AvailableRead() == 0 {
    return portaudio.Complete  // Signal end of playback
}
```

## Usage

```bash
# Build
go build -o play_callback ./pkg/audioplayer/examples/play_callback/main.go

# Play audio
./play_callback music.mp3
./play_callback -device 0 music.flac

# With verbose logging
./play_callback -v music.mp3

# Custom buffer size for different scenarios
./play_callback -buffer 65536 -frames 256 music.mp3   # Lower latency
./play_callback -buffer 524288 -frames 1024 music.mp3 # More stability
```

## API Usage: OpenCallback()

The key difference is using `OpenCallback()` instead of `Open()`:

```go
// Create stream
stream := &portaudio.PaStream{
    OutputParameters: portaudio.PaStreamParameters{
        DeviceIndex:  deviceIdx,
        ChannelCount: channels,
        SampleFormat: portaudio.SampleFmtInt16,
    },
    SampleRate: float64(sampleRate),
}

// Open with callback (not blocking Open())
err := stream.OpenCallback(framesPerBuffer, audioCallback)

// Start stream
err = stream.StartStream()

// ... callback runs automatically ...

// Stop and cleanup
stream.StopStream()
stream.CloseCallback()  // Use CloseCallback(), not Close()
```

## Callback Function Signature

```go
func audioCallback(
    input, output []byte,               // Audio buffers
    frameCount uint,                    // Number of frames to process
    timeInfo *StreamCallbackTimeInfo,   // Timing information
    statusFlags StreamCallbackFlags,    // Status flags (underrun, etc.)
) StreamCallbackResult                  // Continue, Complete, or Abort
```

### Return Values
- `Continue`: Keep stream running, call again
- `Complete`: Finish gracefully, play remaining buffers then stop
- `Abort`: Stop immediately, discard buffered data

### Status Flags
- `InputUnderflow`: Lost input data
- `InputOverflow`: Discarded input data
- `OutputUnderflow`: Buffer underrun occurred
- `OutputOverflow`: Buffer overrun occurred
- `PrimingOutput`: Initial output generation

## Real-Time Callback Constraints

⚠️ **The callback runs in a real-time thread. Avoid:**
- Memory allocation (`make()`, `append()`, etc.)
- File I/O (`os.Open()`, `fmt.Printf()`, `slog`)
- Network I/O
- Mutex locks (use lock-free structures)
- System calls that may block
- Any operation with unbounded execution time

✅ **Safe to use:**
- Pre-allocated buffers
- Atomic operations
- Lock-free data structures (like our ringbuffer)
- Simple arithmetic and copies
- Direct memory access

## Performance Comparison

| Metric | Blocking I/O | Callback Mode |
|--------|--------------|---------------|
| Latency | ~20-50ms | ~5-10ms |
| CPU Usage | Moderate | Lower |
| Jitter | Higher | Lower |
| Underrun Risk | Lower | Higher (if callback too slow) |
| Complexity | Simple | Requires RT programming |

## When to Use Callback Mode

**Use callbacks when:**
- ✅ Low latency is critical
- ✅ Real-time audio processing needed
- ✅ Professional audio application
- ✅ Audio synthesis or effects
- ✅ High performance required

**Use blocking I/O when:**
- ✅ Simplicity is more important than latency
- ✅ File playback (not real-time)
- ✅ Prototyping or learning
- ✅ Don't need sub-20ms latency

## Troubleshooting

### Audio Glitches/Dropouts
- Increase buffer size: `-buffer 524288`
- Increase frames per buffer: `-frames 1024`
- Check system load (callback may be interrupted)

### High Latency
- Decrease buffer size: `-buffer 65536`
- Decrease frames: `-frames 256`
- Ensure callback is fast enough

### Underruns
- Producer may be too slow (decoder performance)
- Increase ringbuffer size
- Check disk I/O performance

## Implementation Notes

### Producer/Consumer Synchronization
- Producer: Standard goroutine, fills ringbuffer
- Consumer: PortAudio RT thread, pulls from ringbuffer via callback
- Coordination: Lock-free ringbuffer with atomic operations
- Completion: Atomic flag signals producer finished

### Memory Management
- All buffers pre-allocated before playback starts
- No allocations in callback
- Zero-copy using `ReadSlices()` when possible

### Error Handling
- Producer errors: Log and signal completion
- Callback errors: Return `Abort` to stop stream
- Graceful degradation: Output silence on underrun

## See Also

- [PortAudio Callback Documentation](http://www.portaudio.com/docs/v19-doxydocs/writing_a_callback.html)
- [Ringbuffer Zero-Copy Methods](../../../ringbuffer/README.md#zero-copy-methods)
- [Blocking I/O Example](../play/README.md)
- [Thread Safety Analysis](../../../../THREAD_SAFETY_ANALYSIS.md)
