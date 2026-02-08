# Claude Code Guidelines for musictools

**Project**: Lock-free audio ringbuffer library with real-time playback capabilities
**Language**: Go 1.25.2
**Architecture**: SPSC (Single-Producer Single-Consumer) pattern
**Status**: Production-ready (Thread Safety: 9.5/10)

---

## Project Overview

This is a high-performance audio processing library implementing lock-free ringbuffers for real-time audio streaming. The project includes:
- Lock-free byte and frame-based ringbuffers
- Multi-format audio decoders (MP3, FLAC, WAV)
- PortAudio-based playback with callback mode
- Sample rate transformation with SoXR
- CLI tools for audio playback and format conversion

---

## Architecture Principles

### 1. SPSC Pattern (Critical)
**All ringbuffers use Single-Producer Single-Consumer pattern:**
- ONE goroutine writes (producer)
- ONE PortAudio callback reads (consumer - runs in C thread!)
- NO mutexes in hot paths - only atomic operations
- Producer and consumer NEVER swap roles

### 2. Thread Safety Model
```
Producer Goroutine ──writes──> RingBuffer ──reads──> Audio Callback (C thread)
        ↓                                                    ↓
  atomic counters                                    atomic counters
  (producedSamples)                                  (playedSamples)
```

**Key invariants:**
- Audio callback runs in C thread with real-time constraints
- No allocations in audio callback
- No blocking operations in audio callback
- Use atomic operations for shared state
- Deep copy slices when storing in ringbuffers

### 3. Package Structure
```
pkg/types/                # Shared interfaces and types (USE THIS for cross-package types)
pkg/decoders/             # Audio format decoders
  ├── factory.go          # Decoder factory (NewDecoder function)
  ├── mp3/                # MP3 decoder
  ├── flac/               # FLAC decoder
  └── wav/                # WAV decoder
pkg/ringbuffer/           # Lock-free byte ringbuffer
pkg/audioframeringbuffer/ # Lock-free frame ringbuffer
pkg/audioframe/           # Frame serialization (12-byte header)
pkg/audioplayer/          # Audio player implementation (legacy)
internal/fileplayer/      # Production FilePlayer (SPSC, 9.5/10 safety)
cmd/                      # CLI commands (thin layer, pure glue code)
  ├── root.go             # Root command
  ├── fileplayer.go       # Playlist command
  ├── player.go           # Simple player command
  └── transform.go        # Sample rate transformation
```

**Architecture Principle**: Keep cmd/ thin - only CLI concerns (flags, signals, logging).
Core implementations belong in pkg/ (public API) or internal/ (private to project).

---

## Critical Design Decisions

### ✅ DO: Correct Patterns

1. **Always use decoder factory for creating decoders**
   ```go
   // ✅ CORRECT - single source of truth
   import "musictools/pkg/decoders"
   decoder, err := decoders.NewDecoder(fileName)

   // ❌ WRONG - duplicating format detection logic
   switch ext {
   case ".mp3": decoder = mp3.NewDecoder()
   case ".flac": decoder = flac.NewDecoder()
   }
   ```

2. **Always use pkg/types for shared interfaces**
   ```go
   // ✅ CORRECT
   import "musictools/pkg/types"
   var decoder types.AudioDecoder

   // ❌ WRONG - don't define interfaces in implementation packages
   var decoder audioplayer.AudioDecoder
   ```

3. **Use internal/fileplayer for production audio playback**
   ```go
   // ✅ CORRECT - production-ready with 9.5/10 safety
   import "musictools/internal/fileplayer"
   player := fileplayer.NewFilePlayer(deviceIdx, capacity, frames, samples)

   // ❌ WRONG - don't reimplement FilePlayer in cmd/
   type MyPlayer struct { /* reinventing the wheel */ }
   ```

4. **Use atomic operations for shared state**
   ```go
   // ✅ CORRECT
   producedSamples atomic.Uint64
   currentFrame atomic.Pointer[audioframe.AudioFrame]

   // ❌ WRONG - race conditions
   producedSamples uint64
   currentFrame *audioframe.AudioFrame
   ```

5. **Deep copy when storing in ringbuffers**
   ```go
   // ✅ CORRECT - prevents buffer reuse corruption
   frame.Audio = make([]byte, bytesToWrite)
   copy(frame.Audio, buffer[:bytesToWrite])

   // ❌ WRONG - shallow copy allows corruption
   frame.Audio = buffer[:bytesToWrite]
   ```

6. **Use channel-based signaling for completion**
   ```go
   // ✅ CORRECT - efficient
   <-fp.playbackCompleteChan

   // ❌ WRONG - polling wastes CPU
   for !fp.playbackComplete.Load() {
       time.Sleep(10 * time.Millisecond)
   }
   ```

7. **Track producer AND consumer metrics**
   ```go
   // ✅ CORRECT - accurate monitoring
   producedSamples atomic.Uint64  // In producer
   playedSamples atomic.Uint64    // In callback
   buffered = produced - played

   // ❌ WRONG - only tracks one side
   elapsedSamples atomic.Uint64   // Ambiguous
   ```

### ❌ DON'T: Anti-Patterns

1. **Never allocate in audio callback**
   ```go
   // ❌ WRONG - allocation in real-time thread
   buffer := make([]byte, size)
   ```

2. **Never block in audio callback**
   ```go
   // ❌ WRONG - blocking in real-time thread
   time.Sleep(duration)
   mu.Lock()
   ch <- data
   ```

3. **Never duplicate types across packages**
   ```go
   // ❌ WRONG - put in pkg/types instead
   type PlaybackStatus struct { ... }  // in pkg/audioplayer
   type PlaybackStatus struct { ... }  // in cmd/fileplayer
   ```

4. **Never use regular pointers for shared state between threads**
   ```go
   // ❌ WRONG - race condition
   currentFrame *audioframe.AudioFrame

   // ✅ CORRECT
   currentFrame atomic.Pointer[audioframe.AudioFrame]
   ```

5. **Never poll when you can signal**
   ```go
   // ❌ WRONG - inefficient polling
   for !done.Load() {
       time.Sleep(time.Millisecond)
   }

   // ✅ CORRECT - efficient signaling
   <-doneChan
   ```

---

## Common Operations

### Building
```bash
go build                    # Main binary (musictools)
go build ./...              # All packages
go build -o bin/name ./path # Specific example
```

### Testing
```bash
go test ./...               # All tests
go test -race ./...         # With race detector
go vet ./...                # Static analysis
```

### Running
```bash
./musictools playlist file1.mp3 file2.flac   # Play playlist
./musictools player file.mp3                 # Simple player
./musictools transform input.mp3 --new-samplerate 48000 --out output.wav
```

---

## Code Style

### 1. Use Go 1.21+ Built-ins
```go
min(a, b)           // Instead of custom min function
clear(slice)        // Instead of loop to zero
```

### 2. Fixed-Size Types for Binary Formats
```go
type FrameFormat struct {
    SampleRate    uint32  // Not int
    Channels      uint8   // Not int
    BitsPerSample uint8   // Not int
}
```

### 3. Power-of-2 Sizes for Ringbuffers
```go
size = nextPowerOf2(size)  // Enables bitwise AND for modulo
mask = size - 1
index = position & mask    // Instead of position % size
```

### 4. Descriptive Field Comments
```go
PlayedSamples   uint64 // Samples actually sent to audio output (played)
BufferedSamples uint64 // Samples decoded but not yet played (in-flight)
```

---

## Recent Refactorings (2026-02-08)

### 1. Types Package Creation
**Motivation**: Fixed duplicate types causing data loss
**What moved**: AudioDecoder, PlaybackStatus, PlaybackMonitor, ringbuffer errors
**Impact**: 12 files updated, single source of truth
**Pattern**: Always use pkg/types for shared abstractions

### 2. Thread Safety Improvements
**Changes**:
- atomic.Pointer for currentFrame (was raw pointer)
- Channel-based completion (was polling)
- Verified Stop() mutex protection
- Verified sample counting safety

**Result**: 9.5/10 safety rating, production-ready

### 3. Accurate Playback Tracking
**Changes**:
- Split tracking: producedSamples (producer) + playedSamples (callback)
- Calculate buffered = produced - played
- Display: played (accurate position), buffered (health), elapsed (wall-clock)

**Result**: Real-time accurate playback monitoring

### 4. Code Architecture Refactoring
**Motivation**: Core implementations were in cmd/ (should only contain CLI glue)
**Changes**:
1. Created `pkg/decoders/factory.go` - Single decoder creation function
2. Moved FilePlayer to `internal/fileplayer/` - Production-ready package
3. Reduced cmd/fileplayer.go from 614 to 221 lines (-64%)

**Impact**:
- Eliminated decoder creation duplication
- FilePlayer now properly packaged and reusable
- cmd/ is now pure CLI layer (flags, signals, logging)

**Result**: Clean architecture with proper separation of concerns

---

## Memory Management

### Key Pattern: Pre-allocate, Don't Allocate in Hot Paths
```go
// ✅ CORRECT - allocate once
buffer := make([]byte, bufferSize)
for {
    decoder.DecodeSamples(samples, buffer)  // Reuse buffer
    // Process...
}

// ❌ WRONG - allocates every iteration
for {
    buffer := make([]byte, bufferSize)
    decoder.DecodeSamples(samples, buffer)
}
```

---

## Dependencies

### Core
- `github.com/drgolem/go-portaudio` - Audio I/O (local replace)
- `github.com/drgolem/go-flac` - FLAC decoder (local replace)
- `github.com/drgolem/go-mpg123` - MP3 decoder
- `github.com/spf13/cobra` - CLI framework

### Transform Command
- `github.com/zaf/resample` - SoXR sample rate conversion
- `github.com/youpy/go-wav` - WAV file I/O

### Local Replaces
```
replace github.com/drgolem/go-flac => /Users/val.gridnev/Projects/.../go-flac
replace github.com/drgolem/go-portaudio => /Users/val.gridnev/Projects/.../go-portaudio
```

---

## File Organization

### Never Commit
- Binaries: `musictools`, `play`, `decode`, etc.
- Build artifacts: `*.test`, `*.out`
- IDE files: `.vscode/`, `.idea/` (if not in .gitignore)

### Always Commit
- Source: `*.go`
- Documentation: `*.md`, `README.md`
- Configuration: `go.mod`, `go.sum`
- Examples: `pkg/*/examples/`

---

## Documentation Standards

### 1. Package-Level Documentation
- Every package has godoc comments
- Examples in `examples/` subdirectories
- README.md for complex packages

### 2. Interface Documentation
```go
// AudioDecoder is the common interface for all audio decoders.
// All decoders must implement these methods to provide a consistent API
// for decoding audio files into raw PCM samples.
type AudioDecoder interface {
    // Open opens an audio file for decoding
    Open(fileName string) error
    // ... (document each method)
}
```

### 3. Implementation Documentation
```go
// Decoder wraps go-wav for decoding WAV audio files.
// Implements types.AudioDecoder interface.
type Decoder struct { ... }
```

---

## Performance Considerations

### 1. Lock-Free Wins
- Ringbuffers: >200k frames/sec throughput
- Zero-copy when possible
- Atomic operations only in critical paths

### 2. Allocation Budget
- AudioFrame deep copy: ~493 ns/frame (acceptable)
- Pre-allocate buffers in loops
- Reuse buffers where safe

### 3. Real-Time Constraints
- Audio callback: <10ms to fill buffer
- No GC pressure in hot paths
- Predictable memory access patterns

---

## Testing Strategy

### Unit Tests
- Test ringbuffer wrap-around cases
- Test atomic operation ordering
- Test error conditions

### Integration Tests
- Test decoder → ringbuffer → playback pipeline
- Test format switching
- Test Stop/Wait synchronization

### Race Detection
```bash
go test -race ./...  # Always run before commit
```

---

## Common Issues & Solutions

### Issue: "undefined: types.AudioDecoder"
**Solution**: Import `musictools/pkg/types`, not `musictools/pkg/audioplayer`

### Issue: Data corruption in ringbuffer
**Solution**: Use deep copy for slice data, not shallow copy

### Issue: Playback time inaccurate
**Solution**: Track samples in audio callback, not producer

### Issue: Race detector warnings
**Solution**: Use atomic operations for all cross-thread state

---

## Future Considerations

### If Adding New Features
1. Maintain SPSC pattern - don't add multi-producer/consumer
2. Put shared types in pkg/types
3. Document thread safety model
4. Add tests with race detector
5. Update MEMORY.md with key decisions

### If Modifying Audio Callback
1. Measure impact on real-time performance
2. No allocations
3. No blocking operations
4. Use atomic operations only
5. Test with race detector

---

## Quick Reference

**Good resources**:
- MEMORY.md - Key implementation patterns learned
- THREAD_SAFETY_ANALYSIS.md - Thread safety guarantees
- pkg/decoders/README.md - Decoder API usage
- pkg/types/types.go - Shared interfaces and types

**Build health check**:
```bash
go build ./... && go vet ./... && go test -race ./...
```

**Safety rating**: 9.5/10 ⭐⭐⭐⭐⭐⭐⭐⭐⭐
**Status**: Production-ready with robust thread safety

---

*Last updated: 2026-02-08*
*Created based on comprehensive thread safety analysis and types refactoring session*
