# AudioFrame Ring Buffer

Lock-free single-producer single-consumer (SPSC) ring buffer for AudioFrame objects.

## Features

- ✅ **Lock-free**: Uses atomic operations for thread-safe SPSC pattern
- ✅ **Safe buffer reuse**: Deep copies audio data, callers can reuse buffers
- ✅ **Power-of-2 sizing**: Automatic rounding for efficient modulo operations
- ✅ **Type-safe**: Works directly with AudioFrame structs
- ✅ **Fast**: ~4.9 μs/op for batch writes, ~4.5 μs/op for batch reads
- ✅ **Simple API**: Just `Write()` and `Read()`
- ✅ **Wrap-around handling**: Automatic circular buffer management

## When to Use This

Use `AudioFrameRingBuffer` when you need to:
- **Pass AudioFrame objects between goroutines** (producer/consumer pattern)
- **Buffer decoded audio frames** before processing
- **Implement frame-based processing pipelines**
- **Store metadata-rich audio data** with format information

**Comparison with byte ringbuffer:**
- **AudioFrameRingBuffer**: Stores complete AudioFrame objects with metadata (format, sample count)
- **RingBuffer**: Stores raw bytes only, faster for simple audio streaming

## Usage

### Basic Example

```go
package main

import (
    "fmt"
    "musictools/pkg/audioframe"
    "musictools/pkg/audioframeringbuffer"
)

func main() {
    // Create buffer for 1024 frames (rounded up to power of 2)
    rb := audioframeringbuffer.New(1024)

    // Create audio frames
    frames := []audioframe.AudioFrame{
        {
            Format: audioframe.FrameFormat{
                SampleRate:    44100,
                Channels:      2,
                BitsPerSample: 16,
            },
            SamplesCount: 1024,
            Audio:        make([]byte, 4096), // 1024 samples × 2 channels × 2 bytes
        },
        {
            Format: audioframe.FrameFormat{
                SampleRate:    48000,
                Channels:      1,
                BitsPerSample: 24,
            },
            SamplesCount: 512,
            Audio:        make([]byte, 1536), // 512 samples × 1 channel × 3 bytes
        },
    }

    // Write frames to buffer
    written, err := rb.Write(frames)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Wrote %d frames\n", written)

    fmt.Printf("Available frames: %d\n", rb.AvailableRead())

    // Read frames from buffer
    readFrames, err := rb.Read(2)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Read %d frames\n", len(readFrames))
    fmt.Printf("First frame: %d Hz, %d channels\n",
        readFrames[0].Format.SampleRate,
        readFrames[0].Format.Channels)
}
```

### Producer-Consumer Pattern

```go
import (
    "sync"
    "musictools/pkg/audioframe"
    "musictools/pkg/audioframeringbuffer"
)

func producerConsumerExample() {
    rb := audioframeringbuffer.New(256)
    var wg sync.WaitGroup
    wg.Add(2)

    // Producer goroutine
    go func() {
        defer wg.Done()
        for i := 0; i < 1000; i++ {
            frame := audioframe.AudioFrame{
                Format: audioframe.FrameFormat{
                    SampleRate:    44100,
                    Channels:      2,
                    BitsPerSample: 16,
                },
                SamplesCount: 1024,
                Audio:        make([]byte, 4096),
            }

            // Retry until all frames written (handles partial writes)
            toWrite := []audioframe.AudioFrame{frame}
            for len(toWrite) > 0 {
                written, _ := rb.Write(toWrite)
                toWrite = toWrite[written:]
                // Yield to consumer if partial write
            }
        }
    }()

    // Consumer goroutine
    go func() {
        defer wg.Done()
        received := 0
        for received < 1000 {
            frames, err := rb.Read(10) // Read up to 10 frames
            if err == audioframeringbuffer.ErrInsufficientData {
                // Buffer empty, yield to producer
                continue
            }

            // Process frames
            for _, frame := range frames {
                processFrame(frame)
                received++
            }
        }
    }()

    wg.Wait()
}

func processFrame(frame audioframe.AudioFrame) {
    // Process the audio frame
}
```

### Batch Processing

```go
// Write multiple frames at once (may be partial write)
frames := []audioframe.AudioFrame{frame1, frame2, frame3}
written, err := rb.Write(frames)
if err == audioframeringbuffer.ErrInsufficientSpace {
    // Buffer completely full (0 frames written)
    // Handle backpressure
} else if written < len(frames) {
    // Partial write - some frames written, retry with rest
    remaining := frames[written:]
    // Handle remaining frames...
}

// Read multiple frames at once
readFrames, err := rb.Read(10)
if err == audioframeringbuffer.ErrInsufficientData {
    // Buffer is empty
}

// Process batch
for _, frame := range readFrames {
    // Process each frame
}
```

## API Reference

### New(capacity uint64) *AudioFrameRingBuffer

Creates a new ring buffer with the specified capacity (number of frames).

**Parameters:**
- `capacity`: Desired number of frames (will be rounded up to next power of 2)

**Returns:**
- `*AudioFrameRingBuffer`: New ring buffer instance

**Example:**
```go
rb := audioframeringbuffer.New(1024) // Creates buffer for 1024 frames
fmt.Printf("Actual capacity: %d\n", rb.Size()) // Prints: 1024
```

### Write(frames []AudioFrame) (int, error)

Writes AudioFrames to the buffer. Writes as many frames as possible (partial write).

**Parameters:**
- `frames`: Slice of AudioFrame objects to write

**Returns:**
- `int`: Number of frames actually written (0 to len(frames))
- `error`: `ErrInsufficientSpace` if buffer completely full (0 written), `nil` otherwise

**Notes:**
- Must only be called by producer goroutine
- Copies frames by value into internal buffer
- **Deep copies the Audio slice** - safe to reuse frame buffers after Write returns
- Allows partial writes - may write fewer frames than requested
- Similar to `io.Writer` pattern

**Example:**
```go
written, err := rb.Write(frames)
if err == audioframeringbuffer.ErrInsufficientSpace {
    // Buffer completely full, no frames written
} else if written < len(frames) {
    // Partial write - handle remaining frames
    remaining := frames[written:]
}
```

### Read(numFrames int) ([]AudioFrame, error)

Reads up to `numFrames` from the buffer.

**Parameters:**
- `numFrames`: Maximum number of frames to read

**Returns:**
- `[]AudioFrame`: Slice of frames read (may be fewer than requested)
- `error`: `ErrInsufficientData` if buffer is empty, `nil` otherwise

**Notes:**
- Must only be called by consumer goroutine
- Returns as many frames as available, up to `numFrames`
- Allocates new slice for returned frames

**Example:**
```go
frames, err := rb.Read(10)
if err == audioframeringbuffer.ErrInsufficientData {
    // Buffer is empty
}
// frames may contain 1-10 frames depending on availability
```

### AvailableRead() uint64

Returns the number of frames available for reading.

**Example:**
```go
if rb.AvailableRead() > 0 {
    frames, _ := rb.Read(int(rb.AvailableRead()))
}
```

### AvailableWrite() uint64

Returns the number of frames that can be written.

**Example:**
```go
if rb.AvailableWrite() >= uint64(len(frames)) {
    _ = rb.Write(frames) // Guaranteed to succeed
}
```

### Size() uint64

Returns the total capacity of the buffer (number of frames).

### Reset()

Clears the buffer by resetting read and write positions. Does not zero memory.

## Performance

Benchmarks on Apple M2 Pro:

```
BenchmarkWrite-12    	  230530	  4930 ns/op	  40960 B/op	  10 allocs/op
BenchmarkRead-12     	  271020	  4507 ns/op	  41373 B/op	  10 allocs/op
```

**Throughput:**
- Write: ~203,000 ops/sec (~4.9 μs per batch of 10 frames)
- Read: ~222,000 ops/sec (~4.5 μs per batch of 10 frames)

**Memory:**
- Write: Deep copies Audio slices for safety (1 allocation per frame)
- Read: 1 allocation per frame (for result slice and audio data)

**Note:** Write performance includes deep copy overhead to ensure safety when callers reuse buffers. This is still more than adequate for real-time audio (can handle >200k frame writes/sec).

## Thread Safety

**IMPORTANT:** This is a **Single-Producer Single-Consumer (SPSC)** ring buffer.

- ✅ **One producer goroutine** calling `Write()`
- ✅ **One consumer goroutine** calling `Read()`
- ❌ **Multiple producers** - NOT SAFE
- ❌ **Multiple consumers** - NOT SAFE

**Correct usage:**
```go
// One producer
go func() {
    for {
        rb.Write(frames) // Only this goroutine writes
    }
}()

// One consumer
go func() {
    for {
        rb.Read(10) // Only this goroutine reads
    }
}()
```

**Incorrect usage:**
```go
// ❌ WRONG: Multiple producers
for i := 0; i < 4; i++ {
    go func() {
        rb.Write(frames) // Race condition!
    }()
}
```

## Implementation Details

### Lock-Free Design

Uses atomic operations (`atomic.Uint64`) for `readPos` and `writePos`:
- Producer updates `writePos` atomically after writing
- Consumer updates `readPos` atomically after reading
- No locks or mutexes needed for SPSC pattern

### Power-of-2 Sizing

Buffer size is automatically rounded to next power of 2:
- Enables efficient modulo operation using bitwise AND
- `position & mask` instead of `position % size`
- Significant performance improvement

**Example:**
```go
rb := New(1000)       // Requested: 1000
fmt.Println(rb.Size()) // Actual: 1024 (next power of 2)
```

### Wrap-Around Handling

Automatically handles circular buffer wrap-around:
```
Buffer: [0] [1] [2] [3] [4] [5] [6] [7]
         ↑write              ↑read

After writing 3 frames with wrap-around:
Buffer: [6] [7] [0] [1] [2] [5] [4] [3]
                     ↑write  ↑read
```

Position tracking uses 64-bit counters that never wrap, preventing ABA problem.

## Use Cases

### 1. Frame-Based Audio Processing

Process audio in discrete frames with format metadata:
```go
rb := audioframeringbuffer.New(128)

// Producer decodes audio into frames
go decodeAudioFrames(rb)

// Consumer processes frames with format info
go func() {
    for {
        frames, _ := rb.Read(10)
        for _, frame := range frames {
            // Access format info
            if frame.Format.SampleRate == 44100 {
                processCD(frame)
            } else {
                processHiRes(frame)
            }
        }
    }
}()
```

### 2. Multi-Format Audio Pipeline

Handle varying audio formats dynamically:
```go
// Each frame carries its own format
frame1 := audioframe.AudioFrame{
    Format: audioframe.FrameFormat{
        SampleRate:    44100,
        Channels:      2,
        BitsPerSample: 16,
    },
    // ...
}

frame2 := audioframe.AudioFrame{
    Format: audioframe.FrameFormat{
        SampleRate:    96000,  // Different rate
        Channels:      6,       // Different channels
        BitsPerSample: 24,      // Different depth
    },
    // ...
}

rb.Write([]audioframe.AudioFrame{frame1, frame2})
```

### 3. Decoder to Processor Pipeline

```go
// Decoder produces frames
func decoder(rb *audioframeringbuffer.AudioFrameRingBuffer, filename string) {
    // Decode audio file into frames
    for {
        frame := decodeNextFrame(filename)
        if frame == nil {
            break
        }
        rb.Write([]audioframe.AudioFrame{*frame})
    }
}

// Processor consumes frames
func processor(rb *audioframeringbuffer.AudioFrameRingBuffer) {
    for {
        frames, err := rb.Read(16)
        if err != nil {
            break
        }
        applyEffects(frames)
    }
}
```

## Comparison with Byte RingBuffer

| Feature | AudioFrameRingBuffer | RingBuffer (bytes) |
|---------|---------------------|-------------------|
| **Data Type** | AudioFrame structs | Raw bytes |
| **Metadata** | ✅ Format info included | ❌ No metadata |
| **Use Case** | Frame-based processing | Raw audio streaming |
| **Write Speed** | ~493 ns/frame | ~10 ns/op |
| **Buffer Reuse** | ✅ Safe (deep copy) | ⚠️ Caller managed |
| **Read Allocation** | 1 per frame | 0 (write to buffer) |
| **Format Changes** | ✅ Per-frame | ❌ Must be handled externally |
| **Complexity** | Higher-level | Lower-level |

**Choose AudioFrameRingBuffer when:**
- You need format metadata with each frame
- Processing logic depends on audio format
- Working with frame-based codecs
- Building complex audio pipelines

**Choose byte RingBuffer when:**
- Maximum performance is critical
- Format is constant throughout stream
- Working with raw PCM data
- Simple producer/consumer pattern

## Error Handling

```go
// Check before writing
if rb.AvailableWrite() < uint64(len(frames)) {
    // Handle backpressure
    log.Println("Buffer full, dropping frames")
} else {
    rb.Write(frames)
}

// Handle errors
err := rb.Write(frames)
if err == audioframeringbuffer.ErrInsufficientSpace {
    // Buffer full
}

frames, err := rb.Read(10)
if err == audioframeringbuffer.ErrInsufficientData {
    // Buffer empty
}
```

## See Also

- [AudioFrame](../audioframe/README.md) - Binary serialization for audio frames
- [RingBuffer](../ringbuffer/README.md) - Lock-free byte ring buffer
- [Audio Player](../audioplayer/README.md) - Audio playback implementation
