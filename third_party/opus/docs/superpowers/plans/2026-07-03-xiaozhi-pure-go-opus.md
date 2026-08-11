# Xiaozhi Pure-Go Opus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `xiaozhi-server-go` use this pure-Go Opus fork for `24 kHz / mono / 60 ms` encode/decode without cgo/libopus.

**Architecture:** Extend the existing public encoder with frame-duration and application options, then implement the `24 kHz / mono / 60 ms` path as a CELT-only Code 3 CBR packet containing three `20 ms` superwideband subframes. Add a qrtc-compatible wrapper so `xiaozhi-server-go` can switch imports with minimal code changes while future SILK/Hybrid work can replace the internal strategy behind the same API.

**Tech Stack:** Go 1.24, existing `github.com/AnimeAIChat/opus` package, existing internal CELT encoder/decoder, `go test ./...`, local module replacement from `xiaozhi-server-go` to `C:\workXF\Projects\opus`.

---

## File Structure

`C:\workXF\Projects\opus\encoder.go`

- Add `Application`, `WithApplication`, `WithFrameDuration`, frame-duration validation, and `24 kHz / mono / 60 ms` dispatch.
- Keep current `48 kHz / 20 ms` behavior unchanged.

`C:\workXF\Projects\opus\encoder_test.go`

- Add public API and round-trip tests for the xiaozhi path.
- Keep tests behavior-focused and decode packets with the pure-Go decoder.

`C:\workXF\Projects\opus\compat.go`

- New qrtc-compatible encoder/decoder config, constructors, wrappers, and `Close` no-op methods.

`C:\workXF\Projects\opus\compat_test.go`

- Test the compatibility API against the `24 kHz / mono / 60 ms` round trip.

`C:\workXF\Projects\opus\errors.go`

- Add `errInvalidFrameDuration`.

`C:\workXF\Projects\AnimeAI\xiaozhi-server-go\go.mod`

- Replace `github.com/qrtc/opus-go` with local `github.com/AnimeAIChat/opus`.

`C:\workXF\Projects\AnimeAI\xiaozhi-server-go\src\core\utils\audio.go`

- Switch the import to the pure-Go fork compatibility layer and set `FrameDuration` for encoder creation.

`C:\workXF\Projects\AnimeAI\xiaozhi-server-go\src\core\utils\audio_opus_test.go`

- Add focused tests proving `NewOpusDecoder` and `PCMSlicesToOpusData` use the pure-Go Opus path.

---

### Task 1: Encoder Options

**Files:**
- Modify: `C:\workXF\Projects\opus\encoder.go`
- Modify: `C:\workXF\Projects\opus\encoder_test.go`
- Modify: `C:\workXF\Projects\opus\errors.go`

- [ ] **Step 1: Write the failing option test**

Add this test to `encoder_test.go`:

```go
func TestNewEncoderAcceptsXiaozhiOptions(t *testing.T) {
	encoder, err := NewEncoder(
		WithSampleRate(24000),
		WithChannels(1),
		WithFrameDuration(60*time.Millisecond),
		WithApplication(ApplicationVoIP),
	)
	require.NoError(t, err)

	assert.Equal(t, 24000, encoder.sampleRate)
	assert.Equal(t, 1, encoder.channels)
	assert.Equal(t, 60*time.Millisecond, encoder.frameDuration)
	assert.Equal(t, ApplicationVoIP, encoder.application)
}
```

Add `time` to the test imports.

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
go test . -run TestNewEncoderAcceptsXiaozhiOptions -count=1
```

Expected: FAIL because `WithFrameDuration`, `WithApplication`, `ApplicationVoIP`, `frameDuration`, and `application` do not exist.

- [ ] **Step 3: Implement minimal option API**

In `errors.go`, add:

```go
errInvalidFrameDuration = errors.New("invalid frame duration")
```

In `encoder.go`, add `time` to imports, add application constants, and add fields/options:

```go
type Application int

const (
	ApplicationVoIP Application = iota + 1
	ApplicationAudio
	ApplicationRestrictedLowDelay
)

const defaultFrameDuration = 20 * time.Millisecond

type Encoder struct {
	celtEncoder   celt.Encoder
	sampleRate    int
	channels      int
	bitrate       int
	complexity    int
	frameDuration time.Duration
	application   Application
}

func WithApplication(application Application) EncoderOption {
	return func(e *Encoder) error {
		switch application {
		case ApplicationVoIP, ApplicationAudio, ApplicationRestrictedLowDelay:
			e.application = application
			return nil
		default:
			return errUnsupportedConfigurationMode
		}
	}
}

func WithFrameDuration(duration time.Duration) EncoderOption {
	return func(e *Encoder) error {
		switch duration {
		case 20 * time.Millisecond, 60 * time.Millisecond:
			e.frameDuration = duration
			return nil
		default:
			return errInvalidFrameDuration
		}
	}
}
```

Update `NewEncoder` defaults:

```go
frameDuration: defaultFrameDuration,
application:   ApplicationAudio,
```

Update `WithSampleRate` to allow `24000` and `48000`, then validate supported combinations after options are applied:

```go
func (e *Encoder) validateConfiguration() error {
	switch {
	case e.sampleRate == 48000 && e.frameDuration == 20*time.Millisecond:
		return nil
	case e.sampleRate == 24000 && e.channels == 1 && e.frameDuration == 60*time.Millisecond:
		return nil
	default:
		return errUnsupportedConfigurationMode
	}
}
```

Call `validateConfiguration` at the end of `NewEncoder`.

- [ ] **Step 4: Run the option test and verify GREEN**

Run:

```powershell
go test . -run TestNewEncoderAcceptsXiaozhiOptions -count=1
```

Expected: PASS.

- [ ] **Step 5: Run existing encoder tests**

Run:

```powershell
go test . -run TestNewEncoder -count=1
```

Expected: PASS.

---

### Task 2: 24 kHz Mono 60 ms Encoding

**Files:**
- Modify: `C:\workXF\Projects\opus\encoder.go`
- Modify: `C:\workXF\Projects\opus\encoder_test.go`

- [ ] **Step 1: Write failing round-trip test**

Add helpers and test to `encoder_test.go`:

```go
const xiaozhiFrameSamples24k = 1440

func TestEncodeXiaozhi24kMono60msRoundTrip(t *testing.T) {
	encoder, err := NewEncoder(
		WithSampleRate(24000),
		WithChannels(1),
		WithFrameDuration(60*time.Millisecond),
		WithApplication(ApplicationVoIP),
		WithBitrate(24000),
	)
	require.NoError(t, err)

	decoder, err := NewDecoderWithOutput(24000, 1)
	require.NoError(t, err)

	pcm := testEncoderSineS16LEAtRate(24000, xiaozhiFrameSamples24k, 440)
	packet := make([]byte, maxOpusFrameSize)

	n, err := encoder.Encode(pcm, packet)
	require.NoError(t, err)
	require.Greater(t, n, 2)

	toc := tableOfContentsHeader(packet[0])
	assert.Equal(t, configurationModeCELTOnly, toc.configuration().mode())
	assert.Equal(t, BandwidthSuperwideband, toc.configuration().bandwidth())
	assert.Equal(t, frameDuration20ms, toc.configuration().frameDuration())
	assert.Equal(t, frameCodeArbitraryFrames, toc.frameCode())

	isVBR, hasPadding, frameCount := parseFrameCountByte(packet[1])
	assert.False(t, isVBR)
	assert.False(t, hasPadding)
	assert.Equal(t, byte(3), frameCount)

	out := make([]int16, xiaozhiFrameSamples24k)
	samples, err := decoder.DecodeToInt16(packet[:n], out)
	require.NoError(t, err)
	assert.Equal(t, xiaozhiFrameSamples24k, samples)
	assert.Greater(t, vectorEnergyInt16(out), 0.0)
}

func testEncoderSineS16LEAtRate(sampleRate, sampleCount int, freq float64) []byte {
	pcm := make([]byte, sampleCount*2)
	for i := range sampleCount {
		sample := int16(math.Round(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * 16000))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	return pcm
}

func vectorEnergyInt16(x []int16) float64 {
	var e float64
	for _, v := range x {
		e += float64(v) * float64(v)
	}
	return math.Sqrt(e)
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
go test . -run TestEncodeXiaozhi24kMono60msRoundTrip -count=1
```

Expected: FAIL because `Encode` still expects the old 48 kHz 20 ms frame size and does not create Code 3 packets.

- [ ] **Step 3: Implement 24 kHz path**

In `encoder.go`, add constants and route `Encode`:

```go
const (
	xiaozhiSubframeDuration = 20 * time.Millisecond
)
```

Update `Encode`:

```go
if e.sampleRate == 24000 && e.channels == 1 && e.frameDuration == 60*time.Millisecond {
	return e.encode24kMono60ms(in, out)
}
```

Add helper methods:

```go
func (e *Encoder) encode24kMono60ms(in []byte, out []byte) (int, error) {
	if len(in)%2 != 0 {
		return 0, fmt.Errorf("%w: s16le length %d not a multiple of 2", errInvalidInputLength, len(in))
	}
	if len(in) != 2880 {
		return 0, fmt.Errorf("%w: got %d bytes, want 2880", errInvalidFrameSize, len(in))
	}
	frameBytes := e.frameBytesForDuration(xiaozhiSubframeDuration)
	if frameBytes <= 0 || frameBytes > maxOpusFrameSize {
		return 0, fmt.Errorf("%w: %d", errInvalidFrameByteBudget, frameBytes)
	}
	if len(out) < 2+3*frameBytes {
		return 0, errOutBufferTooSmall
	}

	out[0] = byte(celtOnlySuperwideband20msConfig<<3) | byte(frameCodeArbitraryFrames)
	out[1] = byte(3 << 2)

	writeOffset := 2
	for i := 0; i < 3; i++ {
		chunk := in[i*960 : (i+1)*960]
		pcm48 := s16LE24kMono20msToFloat48k(chunk)
		n, err := e.celtEncoder.EncodeFrame(
			[][]float32{pcm48},
			out[writeOffset:writeOffset+frameBytes],
			frameBytes,
			0,
			19,
		)
		if err != nil {
			return 0, err
		}
		if n != frameBytes {
			return 0, fmt.Errorf("%w: cbr subframe wrote %d bytes, want %d", errInvalidFrameByteBudget, n, frameBytes)
		}
		writeOffset += n
	}
	return writeOffset, nil
}

func s16LE24kMono20msToFloat48k(in []byte) []float32 {
	out := make([]float32, 960)
	for i := 0; i < 480; i++ {
		sample := float32(int16(binary.LittleEndian.Uint16(in[i*2:]))) / 32768
		out[i*2] = sample
		out[i*2+1] = sample
	}
	return out
}
```

Add:

```go
const celtOnlySuperwideband20msConfig = 27
```

Add:

```go
func (e *Encoder) frameBytesForDuration(duration time.Duration) int {
	return int(int64(e.bitrate) * int64(duration) / int64(time.Second) / 8)
}
```

Update existing `frameBytes` to call `frameBytesForDuration(e.frameDuration)` for the 48 kHz path or keep the old `20 ms` calculation for current behavior.

- [ ] **Step 4: Run the round-trip test and verify GREEN**

Run:

```powershell
go test . -run TestEncodeXiaozhi24kMono60msRoundTrip -count=1
```

Expected: PASS.

- [ ] **Step 5: Run opus package tests**

Run:

```powershell
go test . -count=1
```

Expected: PASS.

---

### Task 3: qrtc-Compatible API

**Files:**
- Create: `C:\workXF\Projects\opus\compat.go`
- Create: `C:\workXF\Projects\opus\compat_test.go`

- [ ] **Step 1: Write failing compatibility test**

Create `compat_test.go`:

```go
package opus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibilityAPIXiaozhiRoundTrip(t *testing.T) {
	encoder, err := CreateOpusEncoder(&OpusEncoderConfig{
		SampleRate:    24000,
		MaxChannels:   1,
		Application:   ApplicationVoIP,
		FrameDuration: 60 * time.Millisecond,
		Bitrate:       24000,
	})
	require.NoError(t, err)
	defer encoder.Close()

	decoder, err := CreateOpusDecoder(&OpusDecoderConfig{
		SampleRate:  24000,
		MaxChannels: 1,
	})
	require.NoError(t, err)
	defer decoder.Close()

	pcm := testEncoderSineS16LEAtRate(24000, xiaozhiFrameSamples24k, 440)
	packet := make([]byte, maxOpusFrameSize)
	n, err := encoder.Encode(pcm, packet)
	require.NoError(t, err)
	require.Positive(t, n)

	out := make([]byte, xiaozhiFrameSamples24k*2)
	decoded, err := decoder.Decode(packet[:n], out)
	require.NoError(t, err)
	assert.Equal(t, xiaozhiFrameSamples24k*2, decoded)
}
```

- [ ] **Step 2: Run the compatibility test and verify RED**

Run:

```powershell
go test . -run TestCompatibilityAPIXiaozhiRoundTrip -count=1
```

Expected: FAIL because the compatibility identifiers do not exist.

- [ ] **Step 3: Implement compatibility layer**

Create `compat.go`:

```go
package opus

import "time"

type OpusEncoderConfig struct {
	SampleRate    int
	MaxChannels   int
	Application   Application
	FrameDuration time.Duration
	Bitrate       int
}

type OpusDecoderConfig struct {
	SampleRate  int
	MaxChannels int
}

type OpusEncoder struct {
	encoder *Encoder
}

type OpusDecoder struct {
	decoder Decoder
	channels int
}

func CreateOpusEncoder(config *OpusEncoderConfig) (*OpusEncoder, error) {
	if config == nil {
		config = &OpusEncoderConfig{}
	}
	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = 48000
	}
	channels := config.MaxChannels
	if channels == 0 {
		channels = 1
	}
	application := config.Application
	if application == 0 {
		application = ApplicationAudio
	}
	frameDuration := config.FrameDuration
	if frameDuration == 0 {
		frameDuration = defaultFrameDuration
	}
	opts := []EncoderOption{
		WithSampleRate(sampleRate),
		WithChannels(channels),
		WithApplication(application),
		WithFrameDuration(frameDuration),
	}
	if config.Bitrate != 0 {
		opts = append(opts, WithBitrate(config.Bitrate))
	}
	encoder, err := NewEncoder(opts...)
	if err != nil {
		return nil, err
	}
	return &OpusEncoder{encoder: encoder}, nil
}

func (e *OpusEncoder) Encode(in, out []byte) (int, error) {
	return e.encoder.Encode(in, out)
}

func (e *OpusEncoder) Close() error {
	return nil
}

func CreateOpusDecoder(config *OpusDecoderConfig) (*OpusDecoder, error) {
	if config == nil {
		config = &OpusDecoderConfig{}
	}
	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = 48000
	}
	channels := config.MaxChannels
	if channels == 0 {
		channels = 1
	}
	decoder, err := NewDecoderWithOutput(sampleRate, channels)
	if err != nil {
		return nil, err
	}
	return &OpusDecoder{decoder: decoder, channels: channels}, nil
}

func (d *OpusDecoder) Decode(in, out []byte) (int, error) {
	tmp := make([]int16, len(out)/2)
	samples, err := d.decoder.DecodeToInt16(in, tmp)
	if err != nil {
		return 0, err
	}
	sampleCount := samples * d.channels
	if sampleCount*2 > len(out) {
		return 0, errOutBufferTooSmall
	}
	for i, sample := range tmp[:sampleCount] {
		out[i*2] = byte(sample)
		out[i*2+1] = byte(uint16(sample) >> 8)
	}
	return sampleCount * 2, nil
}

func (d *OpusDecoder) Close() error {
	return nil
}
```

- [ ] **Step 4: Run compatibility test and verify GREEN**

Run:

```powershell
go test . -run TestCompatibilityAPIXiaozhiRoundTrip -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full opus tests**

Run:

```powershell
go test ./... -count=1
```

Expected: PASS.

---

### Task 4: xiaozhi-server-go Integration

**Files:**
- Modify: `C:\workXF\Projects\AnimeAI\xiaozhi-server-go\go.mod`
- Modify: `C:\workXF\Projects\AnimeAI\xiaozhi-server-go\src\core\utils\audio.go`
- Create: `C:\workXF\Projects\AnimeAI\xiaozhi-server-go\src\core\utils\audio_opus_test.go`

- [ ] **Step 1: Write failing xiaozhi test**

Create `audio_opus_test.go`:

```go
package utils

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestPureGoOpusXiaozhiPath(t *testing.T) {
	const sampleRate = 24000
	const samples = sampleRate * 60 / 1000

	pcm := make([]byte, samples*2)
	for i := range samples {
		sample := int16(math.Round(math.Sin(2*math.Pi*440*float64(i)/sampleRate) * 16000))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}

	packets, err := PCMSlicesToOpusData([][]byte{pcm}, sampleRate, 1, 24000)
	if err != nil {
		t.Fatalf("PCMSlicesToOpusData: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("packet count = %d, want 1", len(packets))
	}

	decoder, err := NewOpusDecoder(&OpusDecoderConfig{SampleRate: sampleRate, MaxChannels: 1})
	if err != nil {
		t.Fatalf("NewOpusDecoder: %v", err)
	}
	defer decoder.Close()

	decoded, err := decoder.Decode(packets[0])
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) != len(pcm) {
		t.Fatalf("decoded bytes = %d, want %d", len(decoded), len(pcm))
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```powershell
go test ./src/core/utils -run TestPureGoOpusXiaozhiPath -count=1
```

Expected before integration: FAIL or still build through `github.com/qrtc/opus-go`, which proves the test is not using the pure-Go fork yet.

- [ ] **Step 3: Switch dependency**

In `go.mod`, change:

```go
github.com/qrtc/opus-go v0.0.1
```

to:

```go
github.com/AnimeAIChat/opus v0.0.0
```

Add local replace:

```go
replace github.com/AnimeAIChat/opus => ../../opus
```

In `audio.go`, change:

```go
opus "github.com/qrtc/opus-go"
```

to:

```go
opus "github.com/AnimeAIChat/opus"
```

In every `CreateOpusEncoder` call, set `FrameDuration: 60 * time.Millisecond`, import `time`, and pass `Bitrate` when the caller provided one:

```go
encoder, err := opus.CreateOpusEncoder(&opus.OpusEncoderConfig{
	SampleRate:    sampleRate,
	MaxChannels:   channels,
	Application:   opus.ApplicationVoIP,
	FrameDuration: 60 * time.Millisecond,
	Bitrate:       bitrate,
})
```

- [ ] **Step 4: Run xiaozhi utility test and verify GREEN**

Run:

```powershell
go test ./src/core/utils -run TestPureGoOpusXiaozhiPath -count=1
```

Expected: PASS.

- [ ] **Step 5: Run broader xiaozhi tests**

Run:

```powershell
go test ./src/core/utils ./src/core -count=1
```

Expected: PASS. If unrelated tests fail, capture the failure and inspect whether the pure-Go Opus change caused it.

---

### Task 5: Final Verification

**Files:**
- Verify only; no planned edits.

- [ ] **Step 1: Run opus full tests**

Run:

```powershell
go test ./... -count=1
```

Expected: PASS in `C:\workXF\Projects\opus`.

- [ ] **Step 2: Prove xiaozhi no longer imports qrtc**

Run:

```powershell
rg -n "github.com/qrtc/opus-go|qrtc/opus-go" .
```

Expected: no matches in `C:\workXF\Projects\AnimeAI\xiaozhi-server-go` except historical comments if any are intentionally kept.

- [ ] **Step 3: Run xiaozhi tests**

Run:

```powershell
go test ./src/core/utils -count=1
```

Expected: PASS.

- [ ] **Step 4: Inspect final git status**

Run in both repositories:

```powershell
git status --short --branch
```

Expected: only intentional changes remain.

- [ ] **Step 5: Summarize**

Report:

- Which Opus files changed.
- Which xiaozhi files changed.
- Exact verification commands and results.
- Remaining limitation: MVP uses CELT-only Code 3 packets for `60 ms`; true SILK encoder remains future work.
