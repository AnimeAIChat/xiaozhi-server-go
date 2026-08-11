// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package opus

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/AnimeAIChat/opus/internal/celt"
)

const (
	defaultBitrate       = 24000
	minBitrate           = 6000
	maxBitrate           = 510000
	frame20msDuration    = 20 * time.Millisecond
	defaultFrameDuration = frame20msDuration
)

// celtOnlyFullband20msConfig is the TOC config number (bits 3..7) for
// CELT-only, fullband, 20 ms frames per RFC 6716 Table 2. The mono/stereo bit
// is separate (bit 2 of the TOC) and not part of this constant.
const celtOnlyFullband20msConfig = 31

// celtOnlyNarrowband20msConfig is the TOC config number for CELT-only,
// narrowband, 20 ms frames.
const celtOnlyNarrowband20msConfig = 19

// celtOnlyWideband20msConfig is the TOC config number for CELT-only,
// wideband, 20 ms frames.
const celtOnlyWideband20msConfig = 23

// celtOnlySuperwideband20msConfig is the TOC config number for CELT-only,
// superwideband, 20 ms frames.
const celtOnlySuperwideband20msConfig = 27

const (
	celt60msSubframeDuration = 20 * time.Millisecond
	celt60msSubframeCount    = 3
)

// Application selects the intended Opus encoder use case.
type Application int

const (
	ApplicationVoIP Application = iota + 1
	ApplicationAudio
	ApplicationRestrictedLowDelay
)

// Encoder encodes PCM into Opus packets.
type Encoder struct {
	celtEncoder   celt.Encoder
	sampleRate    int
	channels      int
	bitrate       int
	complexity    int
	frameDuration time.Duration
	application   Application
}

// EncoderOption configures an Encoder during construction.
//
// Options are applied in the order they are passed to NewEncoder. Each option
// returns an error if the requested value is unsupported by the current
// encoder slice, so callers can detect unsupported configurations at
// construction time rather than at first encode.
type EncoderOption func(*Encoder) error

// WithSampleRate sets the input sample rate in Hz.
func WithSampleRate(rate int) EncoderOption {
	return func(e *Encoder) error {
		if isSupportedInputSampleRate(rate) {
			e.sampleRate = rate

			return nil
		}

		return errInvalidSampleRate
	}
}

// WithChannels sets the channel count (1 for mono, 2 for stereo).
func WithChannels(channels int) EncoderOption {
	return func(e *Encoder) error {
		if channels < 1 || channels > 2 {
			return errInvalidChannelCount
		}
		e.channels = channels

		return nil
	}
}

// WithBitrate sets the target bitrate in bits per second. Valid range is
// 6000 to 510000.
func WithBitrate(bps int) EncoderOption {
	return func(e *Encoder) error {
		if bps < minBitrate || bps > maxBitrate {
			return fmt.Errorf("%w: %d", errBitrateOutOfRange, bps)
		}
		e.bitrate = bps

		return nil
	}
}

// WithComplexity sets the encoder complexity on the standard Opus 0..10
// scale. The current CELT encoder does not vary behavior by complexity, but
// the public API accepts the value for future expansion.
func WithComplexity(complexity int) EncoderOption {
	return func(e *Encoder) error {
		if complexity < 0 || complexity > 10 {
			return fmt.Errorf("%w: %d", errInvalidComplexity, complexity)
		}
		e.complexity = complexity

		return nil
	}
}

// WithApplication sets the intended encoder application.
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

// WithFrameDuration sets the duration represented by one call to Encode.
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

// NewEncoder creates a new Opus encoder with the supplied options.
//
// Defaults: 48 kHz, mono, 24 kbit/s, complexity 0. Pass options to override
// any of these. The current implementation supports 48 kHz, 1 or 2 channels,
// 20 ms CELT-only packets, and a CELT-only 60 ms bridge for Opus input sample
// rates. Transient detection and SILK encoding will land in follow-up PRs.
func NewEncoder(opts ...EncoderOption) (*Encoder, error) {
	encoder := &Encoder{
		celtEncoder:   celt.NewEncoder(),
		sampleRate:    celtSampleRate,
		channels:      1,
		bitrate:       defaultBitrate,
		complexity:    0,
		frameDuration: defaultFrameDuration,
		application:   ApplicationAudio,
	}

	for _, opt := range opts {
		if err := opt(encoder); err != nil {
			return nil, err
		}
	}

	if err := encoder.validateConfiguration(); err != nil {
		return nil, err
	}

	return encoder, nil
}

// SetBitrate updates the target bitrate in bits per second.
func (e *Encoder) SetBitrate(bps int) error {
	return WithBitrate(bps)(e)
}

// SetComplexity updates the encoder complexity on the standard Opus 0..10
// scale.
func (e *Encoder) SetComplexity(complexity int) error {
	return WithComplexity(complexity)(e)
}

func (e *Encoder) validateConfiguration() error {
	switch {
	case e.sampleRate == celtSampleRate && e.frameDuration == frame20msDuration:
		return nil
	case isSupportedInputSampleRate(e.sampleRate) && e.frameDuration == 60*time.Millisecond:
		return nil
	default:
		return errUnsupportedConfigurationMode
	}
}

// Encode encodes S16LE PCM into a single Opus packet.
//
// The input must contain exactly one configured S16LE frame.
func (e *Encoder) Encode(in []byte, out []byte) (int, error) {
	if len(in)%2 != 0 {
		return 0, fmt.Errorf("%w: s16le length %d not a multiple of 2", errInvalidInputLength, len(in))
	}
	if e.frameDuration == 60*time.Millisecond {
		return e.encodeCELT60ms(in, out)
	}

	expectedSamples := e.frameSampleCount() * e.channels
	if len(in)/2 != expectedSamples {
		return 0, fmt.Errorf("%w: got %d samples, want %d", errInvalidFrameSize, len(in)/2, expectedSamples)
	}

	pcm := make([]float32, len(in)/2)
	for i := range pcm {
		sample := int16(binary.LittleEndian.Uint16(in[i*2:])) //nolint:gosec // G115: little-endian s16 round-trip.
		pcm[i] = float32(sample) / 32768
	}

	return e.EncodeFloat32(pcm, out)
}

func (e *Encoder) encodeCELT60ms(in []byte, out []byte) (int, error) {
	if len(in)%2 != 0 {
		return 0, fmt.Errorf("%w: s16le length %d not a multiple of 2", errInvalidInputLength, len(in))
	}
	expectedBytes := e.frameInputByteCount()
	if len(in) != expectedBytes {
		return 0, fmt.Errorf("%w: got %d bytes, want %d", errInvalidFrameSize, len(in), expectedBytes)
	}

	frameBytes := e.frameBytesForDuration(celt60msSubframeDuration)
	if frameBytes <= 0 || frameBytes > maxOpusFrameSize {
		return 0, fmt.Errorf("%w: %d", errInvalidFrameByteBudget, frameBytes)
	}
	packetBytes := 2 + celt60msSubframeCount*frameBytes
	if len(out) < packetBytes {
		return 0, errOutBufferTooSmall
	}

	config, celtBandSampleRate, err := celt20msConfigForInputSampleRate(e.sampleRate)
	if err != nil {
		return 0, err
	}

	out[0] = byte(config<<3) | byte(frameCodeArbitraryFrames)
	if e.channels == 2 {
		out[0] |= 1 << 2
	}
	out[1] = celt60msSubframeCount

	startBand, endBand, err := e.celtEncoder.Mode().BandRangeForSampleRate(celtBandSampleRate)
	if err != nil {
		return 0, err
	}

	writeOffset := 2
	subframeBytes := e.subframeInputByteCount()
	for i := 0; i < celt60msSubframeCount; i++ {
		chunk := in[i*subframeBytes : (i+1)*subframeBytes]
		pcm48, err := s16LEInterleaved20msToFloat48k(chunk, e.sampleRate, e.channels)
		if err != nil {
			return 0, err
		}
		frameOut := out[writeOffset : writeOffset+frameBytes]
		clear(frameOut)

		n, err := e.celtEncoder.EncodeFrame(
			pcm48,
			frameOut,
			frameBytes,
			startBand,
			endBand,
		)
		if err != nil {
			return 0, err
		}
		if n > frameBytes {
			return 0, fmt.Errorf("%w: cbr subframe wrote %d bytes, want at most %d", errInvalidFrameByteBudget, n, frameBytes)
		}

		writeOffset += frameBytes
	}

	return packetBytes, nil
}

func s16LEInterleaved20msToFloat48k(in []byte, sampleRate, channels int) ([][]float32, error) {
	sourceSamples := int(int64(sampleRate) * int64(celt60msSubframeDuration) / int64(time.Second))
	targetSamples := int(int64(celtSampleRate) * int64(celt60msSubframeDuration) / int64(time.Second))
	if sourceSamples == 0 || targetSamples%sourceSamples != 0 {
		return nil, errInvalidSampleRate
	}
	if len(in) != sourceSamples*channels*2 {
		return nil, errInvalidFrameSize
	}

	factor := targetSamples / sourceSamples
	out := make([][]float32, channels)
	for ch := range channels {
		out[ch] = make([]float32, targetSamples)
	}
	for i := 0; i < sourceSamples; i++ {
		for ch := 0; ch < channels; ch++ {
			offset := (i*channels + ch) * 2
			sample := float32(int16(binary.LittleEndian.Uint16(in[offset:]))) / 32768
			for j := 0; j < factor; j++ {
				out[ch][i*factor+j] = sample
			}
		}
	}

	return out, nil
}

func celt20msConfigForInputSampleRate(sampleRate int) (config byte, bandSampleRate int, err error) {
	switch sampleRate {
	case 8000:
		return celtOnlyNarrowband20msConfig, 8000, nil
	case 12000, 16000:
		return celtOnlyWideband20msConfig, 16000, nil
	case 24000:
		return celtOnlySuperwideband20msConfig, 24000, nil
	case 48000:
		return celtOnlyFullband20msConfig, 48000, nil
	default:
		return 0, 0, errInvalidSampleRate
	}
}

// EncodeFloat32 encodes float PCM into a single Opus packet.
//
// The input must contain one 20 ms 48 kHz frame.
func (e *Encoder) EncodeFloat32(in []float32, out []byte) (int, error) {
	if e.sampleRate != celtSampleRate {
		return 0, errInvalidSampleRate
	}

	frameSamples := e.frameSampleCount()
	if len(in) != frameSamples*e.channels {
		return 0, fmt.Errorf("%w: got %d samples, want %d", errInvalidFrameSize, len(in), frameSamples*e.channels)
	}

	channels := splitChannels(in, e.channels, frameSamples)

	frameBytes := e.frameBytes()
	if frameBytes <= 0 || frameBytes > maxOpusFrameSize {
		return 0, fmt.Errorf("%w: %d", errInvalidFrameByteBudget, frameBytes)
	}
	if len(out) < frameBytes+1 {
		return 0, errOutBufferTooSmall
	}
	out[0] = byte(e.tocHeader())
	n, err := e.celtEncoder.EncodeFrame(channels, out[1:frameBytes+1], frameBytes, 0, e.celtEncoder.Mode().BandCount())
	if err != nil {
		return 0, err
	}

	return 1 + n, nil
}

func (e *Encoder) tocHeader() tableOfContentsHeader {
	header := byte(celtOnlyFullband20msConfig << 3)
	header |= byte(frameCodeOneFrame)
	if e.channels == 2 {
		header |= 1 << 2
	}

	return tableOfContentsHeader(header)
}

// splitChannels splits interleaved PCM into per-channel slices.
// For mono, it returns the input directly without allocation.
func splitChannels(in []float32, numChannels, frameSamples int) [][]float32 {
	ch := make([][]float32, numChannels)
	if numChannels == 1 {
		ch[0] = in

		return ch
	}

	for c := range numChannels {
		ch[c] = make([]float32, frameSamples)
		for i := range frameSamples {
			ch[c][i] = in[i*numChannels+c]
		}
	}

	return ch
}

func (e *Encoder) frameBytes() int {
	return e.frameBytesForDuration(frame20msDuration)
}

func (e *Encoder) frameBytesForDuration(duration time.Duration) int {
	return int(int64(e.bitrate) * int64(duration) / int64(time.Second) / 8)
}

func (e *Encoder) frameSampleCount() int {
	return int(int64(celtSampleRate) * int64(frame20msDuration) / int64(time.Second))
}

func (e *Encoder) frameInputByteCount() int {
	samplesPerChannel := int(int64(e.sampleRate) * int64(e.frameDuration) / int64(time.Second))

	return samplesPerChannel * e.channels * 2
}

func (e *Encoder) subframeInputByteCount() int {
	samplesPerChannel := int(int64(e.sampleRate) * int64(celt60msSubframeDuration) / int64(time.Second))

	return samplesPerChannel * e.channels * 2
}

func isSupportedInputSampleRate(sampleRate int) bool {
	switch sampleRate {
	case 8000, 12000, 16000, 24000, celtSampleRate:
		return true
	default:
		return false
	}
}
