# Xiaozhi Pure-Go Opus Design

## Goal

Make `xiaozhi-server-go` able to replace its current cgo/libopus dependency with this pure-Go Opus package for the immediate production path: `24 kHz`, mono, `60 ms` Opus audio packets for outbound TTS and the existing pure-Go decoder for inbound Opus.

The design must also leave clear extension points for a future fuller libopus-compatible encoder, including true SILK-only speech encoding, Hybrid mode, CTL behavior, FEC, DTX, and broader frame/sample-rate support.

## Current State

The fork already has a pure-Go Opus decoder that accepts output rates `8000`, `12000`, `16000`, `24000`, and `48000`, parses Code 3 multi-frame packets, and decodes CELT-only, SILK-only, and Hybrid packet modes.

The public encoder currently supports only `48 kHz`, mono or stereo, `20 ms`, CELT-only fullband packets. It does not expose frame duration or application options, and it does not have a SILK encoder.

`xiaozhi-server-go` currently uses `github.com/qrtc/opus-go`, which wraps system `libopus` through cgo. Its required path is:

- `CreateOpusDecoder` and `Decode` for inbound Opus packets.
- `CreateOpusEncoder` and `Encode` for outbound PCM-to-Opus packets.
- Default outbound audio parameters: `24 kHz`, mono, `60 ms`.

## MVP Encoding Strategy

Because this fork does not yet contain a SILK encoder, the first deliverable will encode `24 kHz / mono / 60 ms` as a standards-compliant CELT-only Opus multi-frame packet:

1. Accept one `60 ms` PCM frame at the configured API rate. For `24 kHz` mono S16LE this is `1440` samples, `2880` bytes.
2. Split it into three `20 ms` chunks of `480` samples each.
3. Convert each chunk to float PCM and upsample from `24 kHz` to the CELT internal `48 kHz`, producing `960` samples.
4. Encode each chunk with the existing internal CELT encoder using the `24 kHz` bandwidth band range, producing CELT-only superwideband `20 ms` frames.
5. Pack the three frames into one Opus Code 3 CBR packet:
   - TOC: CELT-only superwideband `20 ms`, mono, frame-count code 3.
   - Frame count byte: CBR, no padding, three frames.
   - Payload: three equal-size encoded CELT frames.

This produces a valid Opus packet with `60 ms` duration and avoids cgo. It is not a substitute for true SILK speech coding quality; it is a compatible pure-Go bridge for the xiaozhi path until SILK encoding is implemented.

## Public API

Extend the existing option-style encoder API without breaking current callers:

```go
encoder, err := opus.NewEncoder(
    opus.WithSampleRate(24000),
    opus.WithChannels(1),
    opus.WithFrameDuration(60*time.Millisecond),
    opus.WithApplication(opus.ApplicationVoIP),
)
```

New exported identifiers:

- `type Application int`
- `const ApplicationVoIP`, `ApplicationAudio`, `ApplicationRestrictedLowDelay`
- `func WithApplication(Application) EncoderOption`
- `func WithFrameDuration(time.Duration) EncoderOption`

The MVP accepts:

- Sample rate: `24000` for the new xiaozhi path and existing `48000` behavior.
- Channels: `1` for the xiaozhi MVP and existing `1`/`2` behavior for `48 kHz`.
- Frame duration: `20 ms` for existing behavior and `60 ms` for `24 kHz` mono.
- Application: accepted and stored for future mode decisions. In the MVP, `ApplicationVoIP` does not yet force SILK because SILK encode is not implemented.

Unsupported combinations return explicit errors at construction or encode time.

## qrtc Compatibility Layer

Add a thin compatibility surface that matches the current xiaozhi usage shape:

```go
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

func CreateOpusEncoder(*OpusEncoderConfig) (*OpusEncoder, error)
func CreateOpusDecoder(*OpusDecoderConfig) (*OpusDecoder, error)
```

`OpusEncoder.Encode(in, out []byte) (int, error)` and `OpusDecoder.Decode(in, out []byte) (int, error)` should delegate to the native pure-Go encoder/decoder. `Close` is a no-op for API compatibility.

This layer lets `xiaozhi-server-go` switch imports with minimal code changes while the main Pion-style API remains the long-term API.

## Data Flow

Outbound xiaozhi TTS path:

```text
MP3/WAV -> PCM slices at 24 kHz mono -> Opus compatibility encoder
       -> 60 ms Opus packets -> websocket binary frames -> client
```

Inbound xiaozhi audio path:

```text
websocket Opus packet -> compatibility decoder -> 24 kHz mono S16LE PCM -> ASR queue
```

## Error Handling

Return existing package errors where possible:

- Invalid sample rate: `errInvalidSampleRate`
- Invalid channel count: `errInvalidChannelCount`
- Invalid frame duration: new `errInvalidFrameDuration`
- Invalid PCM byte length or sample count: `errInvalidFrameSize`
- Output buffer too small: `errOutBufferTooSmall`

The compatibility layer should wrap none of these errors unless it adds actionable context needed by callers.

## Testing

Unit tests must be added before implementation:

- `NewEncoder(WithSampleRate(24000), WithChannels(1), WithFrameDuration(60*time.Millisecond))` succeeds.
- The same encoder rejects a PCM frame shorter or longer than `2880` bytes.
- Encoding a deterministic `24 kHz / mono / 60 ms` sine frame yields a non-empty packet.
- The packet TOC is CELT-only superwideband `20 ms` with Code 3.
- The Code 3 frame-count byte reports CBR, no padding, three frames.
- The pure-Go decoder with `NewDecoderWithOutput(24000, 1)` decodes that packet and returns `1440` samples per channel.
- The qrtc-compatible `CreateOpusEncoder` and `CreateOpusDecoder` round-trip a packet through the same `24 kHz / mono / 60 ms` path.

Integration verification in `xiaozhi-server-go` must prove the project can build and run unit tests without `github.com/qrtc/opus-go` or cgo-required Opus headers.

## Future Full libopus Compatibility

The MVP must not hard-code itself as the final encoder architecture. It should keep mode selection isolated so later work can route through:

- CELT-only encoder for music and low-delay paths.
- SILK-only encoder for NB/MB/WB speech and `10/20/40/60 ms` frames.
- Hybrid encoder for SWB/FB speech at medium bitrates.

The future SILK encoder should be implemented under `internal/silk` by porting the official source in this order:

1. Encoder state/control structures and `silk_Encode` API flow from `silk/enc_API.c`.
2. Packet size, bitrate, and internal sample-rate control from `silk/control_codec.c` and `silk/control_SNR.c`.
3. Frame encoding from `silk/fixed/encode_frame_FIX.c` or the floating-point path.
4. Index and pulse encoding using the existing decoder tables as shared source where possible.
5. Opus top-level mode selection from `src/opus_encoder.c`.

The API added for the MVP should remain valid when true SILK/HYBRID mode selection is added.

## Acceptance Criteria

- `go test ./...` passes in `C:\workXF\Projects\opus`.
- A `24 kHz / mono / 60 ms` S16LE frame can be encoded and decoded purely in Go.
- The encoded packet is a valid Opus Code 3 packet with three `20 ms` frames.
- `xiaozhi-server-go` can depend on the local pure-Go fork and no longer imports `github.com/qrtc/opus-go`.
- A focused test in `xiaozhi-server-go` covers `utils.NewOpusDecoder` and `utils.PCMSlicesToOpusData` through the pure-Go compatibility layer.
