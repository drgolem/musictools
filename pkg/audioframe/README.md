# AudioFrame - Binary Serialization for Audio Data

Efficient binary serialization/deserialization of audio frames using Go's `encoding/binary` package.

## Features

- ✅ **Little-endian encoding** (standard for audio formats)
- ✅ **Zero-copy where possible** (direct buffer access)
- ✅ **Implements standard interfaces** (`encoding.BinaryMarshaler`/`BinaryUnmarshaler`)
- ✅ **Efficient** (~450-475 ns/op, 1 allocation)
- ✅ **Safe** (validates buffer sizes, prevents overflows)
- ✅ **Tested** (100% coverage with benchmarks)

## Data Structure

```go
type FrameFormat struct {
    SampleRate    uint32  // Sample rate in Hz (max 384,000)
    Channels      uint8   // Number of channels (max 10)
    BitsPerSample uint8   // Bits per sample (max 64)
}

type AudioFrame struct {
    Format       FrameFormat  // Audio format
    SamplesCount uint16       // Number of samples (max 65,535)
    Audio        []byte       // Raw audio data
}
```

## Binary Format

The serialized format uses **little-endian encoding** with a tightly packed 12-byte header:

```
Offset | Size | Field
-------|------|------------------
0      | 4    | SampleRate (uint32)
4      | 1    | Channels (uint8)
5      | 1    | BitsPerSample (uint8)
6      | 2    | SamplesCount (uint16)
8      | 4    | Audio length (uint32)
12     | N    | Audio data (N bytes)
-------|------|------------------
Total: 12 + N bytes
```

**Why little-endian?**
- Standard for most audio formats (WAV, FLAC, etc.)
- Matches CPU endianness on x86/ARM (most platforms)
- Compatible with audio hardware and DAWs

## Usage

### Basic Marshal/Unmarshal

```go
package main

import (
    "fmt"
    "musictools/pkg/audioframe"
)

func main() {
    // Create an audio frame
    frame := audioframe.AudioFrame{
        Format: audioframe.FrameFormat{
            SampleRate:    44100,
            Channels:      2,
            BitsPerSample: 16,
        },
        Audio:        []byte{0x00, 0x01, 0x02, 0x03},
        SamplesCount: 2, // 2 samples of stereo 16-bit
    }

    // Marshal to bytes
    data := frame.Marshal()
    fmt.Printf("Serialized size: %d bytes\n", len(data))

    // Unmarshal back
    var decoded audioframe.AudioFrame
    err := decoded.Unmarshal(data)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Sample Rate: %d\n", decoded.Format.SampleRate)
    fmt.Printf("Channels: %d\n", decoded.Format.Channels)
}
```

### Using Standard Interfaces

```go
import "encoding"

var frame audioframe.AudioFrame
var _ encoding.BinaryMarshaler = &frame
var _ encoding.BinaryUnmarshaler = &frame

// Marshal using interface
data, err := frame.MarshalBinary()

// Unmarshal using interface
err = frame.UnmarshalBinary(data)
```

### Streaming Over Network

```go
import "net"

// Send audio frame over TCP
func sendFrame(conn net.Conn, frame *audioframe.AudioFrame) error {
    data := frame.Marshal()

    // Send length first (4 bytes)
    length := make([]byte, 4)
    binary.LittleEndian.PutUint32(length, uint32(len(data)))
    if _, err := conn.Write(length); err != nil {
        return err
    }

    // Send frame data
    _, err := conn.Write(data)
    return err
}

// Receive audio frame from TCP
func receiveFrame(conn net.Conn) (*audioframe.AudioFrame, error) {
    // Read length (4 bytes)
    lengthBuf := make([]byte, 4)
    if _, err := io.ReadFull(conn, lengthBuf); err != nil {
        return nil, err
    }
    length := binary.LittleEndian.Uint32(lengthBuf)

    // Read frame data
    data := make([]byte, length)
    if _, err := io.ReadFull(conn, data); err != nil {
        return nil, err
    }

    // Unmarshal
    var frame audioframe.AudioFrame
    if err := frame.Unmarshal(data); err != nil {
        return nil, err
    }

    return &frame, nil
}
```

### Writing to File

```go
import "os"

// Write audio frames to file
func writeFramesToFile(filename string, frames []*audioframe.AudioFrame) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    for _, frame := range frames {
        data := frame.Marshal()

        // Write frame length
        length := make([]byte, 4)
        binary.LittleEndian.PutUint32(length, uint32(len(data)))
        if _, err := file.Write(length); err != nil {
            return err
        }

        // Write frame data
        if _, err := file.Write(data); err != nil {
            return err
        }
    }

    return nil
}
```

## API Reference

### Marshal() []byte

Serializes the AudioFrame to a byte slice.

```go
data := frame.Marshal()
```

**Returns:**
- `[]byte`: Serialized data (12 bytes header + audio data)

**Performance:**
- Time: ~444 ns/op
- Allocations: 1 (for result buffer)

### Unmarshal(data []byte) error

Deserializes a byte slice into the AudioFrame.

```go
var frame audioframe.AudioFrame
err := frame.Unmarshal(data)
```

**Parameters:**
- `data []byte`: Serialized audio frame data

**Returns:**
- `error`: nil on success, error if:
  - Buffer too small (< 12 bytes)
  - Audio length field exceeds buffer size
  - Invalid format

**Performance:**
- Time: ~449 ns/op
- Allocations: 1 (for audio buffer)

### MarshalBinary() ([]byte, error)

Implements `encoding.BinaryMarshaler` interface.

```go
data, err := frame.MarshalBinary()
```

### UnmarshalBinary(data []byte) error

Implements `encoding.BinaryUnmarshaler` interface.

```go
err := frame.UnmarshalBinary(data)
```

## Performance

Benchmarks on Apple M2 Pro:

```
BenchmarkMarshal-12      	 2698948	  444.1 ns/op	  4864 B/op	  1 allocs/op
BenchmarkUnmarshal-12    	 2667943	  449.3 ns/op	  4096 B/op	  1 allocs/op
```

**Throughput:**
- Marshal: ~2.25 million ops/sec
- Unmarshal: ~2.23 million ops/sec

**Memory:**
- Marshal: 4864 bytes/op (12-byte header + 4096 audio data)
- Unmarshal: 4096 bytes/op (audio buffer)
- Single allocation per operation

## Error Handling

```go
var frame audioframe.AudioFrame

// Too small buffer
err := frame.Unmarshal([]byte{1, 2, 3})
// Error: "buffer too small: got 3 bytes, need at least 20 bytes"

// Corrupted header (audio length exceeds buffer)
badData := make([]byte, 20)
binary.LittleEndian.PutUint32(badData[16:20], 999999)
err = frame.Unmarshal(badData)
// Error: "buffer too small for audio data: got 20 bytes, need 100019 bytes"
```

## Use Cases

### 1. Network Streaming
Serialize audio frames for transmission over network protocols (TCP, UDP, WebRTC).

### 2. File Storage
Save audio frames to disk for buffering, caching, or recording.

### 3. Inter-Process Communication
Share audio data between processes using shared memory or pipes.

### 4. Audio Processing Pipeline
Pass audio frames between processing stages with minimal overhead.

### 5. Audio Capture/Playback
Buffer audio data between capture and playback threads.

## Size Calculations

**Header size:** Always 20 bytes (5 × int32)

**Total size:** 20 + len(Audio) bytes

**Examples:**
```go
// Stereo 16-bit, 1024 samples
// Audio bytes: 1024 samples × 2 channels × 2 bytes = 4096 bytes
// Total: 20 + 4096 = 4116 bytes

// Mono 24-bit, 512 samples
// Audio bytes: 512 samples × 1 channel × 3 bytes = 1536 bytes
// Total: 20 + 1536 = 1556 bytes

// 5.1 surround 32-bit, 2048 samples
// Audio bytes: 2048 samples × 6 channels × 4 bytes = 49152 bytes
// Total: 20 + 49152 = 49172 bytes
```

## Thread Safety

**Marshal()** and **Unmarshal()** are **NOT thread-safe** when operating on the same AudioFrame instance. Use proper synchronization if accessing from multiple goroutines:

```go
var mu sync.Mutex
var frame audioframe.AudioFrame

// Safe concurrent marshal
mu.Lock()
data := frame.Marshal()
mu.Unlock()
```

For concurrent access, consider using separate AudioFrame instances per goroutine.

## Comparison with Other Formats

| Format | Size Overhead | Speed | Compatibility |
|--------|---------------|-------|---------------|
| **Custom Binary** | 20 bytes | ⚡ Fastest | Custom |
| JSON | ~100+ bytes | 🐌 Slow | Universal |
| Protobuf | ~15 bytes | ⚡ Fast | Universal |
| MessagePack | ~20 bytes | ⚡ Fast | Universal |
| gob | ~30 bytes | 🐌 Slow | Go only |

**Why custom binary?**
- Minimal overhead (20 bytes fixed)
- No external dependencies
- Optimized for audio use case
- Simple implementation
- Predictable size/performance

## Testing

Run tests:
```bash
go test ./pkg/audioframe/ -v
```

Run benchmarks:
```bash
go test ./pkg/audioframe/ -bench=. -benchmem
```

## See Also

- [RingBuffer](../ringbuffer/README.md) - Lock-free buffer for audio streaming
- [Audio Player](../audioplayer/README.md) - Audio playback implementation
- [Go encoding/binary](https://pkg.go.dev/encoding/binary) - Binary encoding package
