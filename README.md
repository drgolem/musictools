# musictools

Command-line audio player and converter for MP3, FLAC, WAV, OGG, and Opus files. Built with [audiokit](https://github.com/drgolem/audiokit) and PortAudio.

## Install

```bash
# macOS
brew install portaudio flac mpg123 libvorbis opus

# Linux
sudo apt-get install portaudio19-dev libflac-dev libmpg123-dev libvorbis-dev libopus-dev

# Build
make build
```

## Commands

### play

Play a single audio file.

```bash
musictools play song.mp3
musictools play -d 1 song.flac        # select audio device
musictools play -v song.wav            # verbose logging

# pipe from stdin
some-tool --stdout | musictools play -
```

### playlist

Play multiple files sequentially.

```bash
musictools playlist track1.mp3 track2.flac track3.wav
musictools playlist *.mp3
musictools playlist -d 0 -v music/*.flac
```

### transform

Resample audio and convert to WAV.

```bash
musictools transform input.mp3 --new-samplerate 48000 --out output.wav
musictools transform input.flac --new-samplerate 44100 --mono --out output.wav
```

### samplecut

Extract a time segment from an audio file.

```bash
musictools samplecut --in song.mp3 --start 1m30s --duration 30s --out clip.wav
```

## Supported formats

| Format | Extensions |
|--------|------------|
| MP3 | `.mp3` |
| FLAC | `.flac`, `.fla` |
| WAV | `.wav` |
| OGG Vorbis | `.ogg`, `.oga` |
| Opus | `.opus` |

## Dependencies

- [audiokit](https://github.com/drgolem/audiokit) -- audio player, decoders, ringbuffer
- [go-portaudio](https://github.com/drgolem/go-portaudio) -- PortAudio bindings
- [resample](https://github.com/zaf/resample) -- SoXR sample rate conversion
- [go-wav](https://github.com/youpy/go-wav) -- WAV file I/O
- [cobra](https://github.com/spf13/cobra) -- CLI framework

## License

MIT
