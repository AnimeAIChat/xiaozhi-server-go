// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package opus

import (
	"encoding/binary"
	"fmt"
	"time"

	silkresample "github.com/AnimeAIChat/opus/internal/resample/silk"
)

const (
	compatFrame20ms = 20 * time.Millisecond
	compatFrame60ms = 60 * time.Millisecond

	compatMaxInputSamples20ms = celtSampleRate / 50
	xiaozhiOutputSamples20ms  = celtSampleRate / 50
	xiaozhiSubframeCount      = 3
)

// OpusEncoderConfig configures the compatibility encoder API used by
// github.com/qrtc/opus-go callers.
type OpusEncoderConfig struct {
	SampleRate    int
	MaxChannels   int
	Application   Application
	FrameDuration time.Duration
	Bitrate       int
}

// OpusDecoderConfig configures the compatibility decoder API used by
// github.com/qrtc/opus-go callers.
type OpusDecoderConfig struct {
	SampleRate  int
	MaxChannels int
}

// OpusEncoder wraps Encoder with a qrtc-compatible method surface.
type OpusEncoder struct {
	encoder            *Encoder
	channels           int
	bridge60ms         bool
	sourceSampleRate   int
	resamplers         [encodeMaxChannels]silkresample.Resampler
	resampleInput      [encodeMaxChannels][compatMaxInputSamples20ms]float32
	resampleOutput     [encodeMaxChannels][xiaozhiOutputSamples20ms]float32
	interleavedPCM48k  [encodeFrameSamples * encodeMaxChannels]float32
	framePacketScratch [maxOpusFrameSize]byte
}

// OpusDecoder wraps Decoder with a qrtc-compatible method surface.
type OpusDecoder struct {
	decoder  Decoder
	channels int
}

// CreateOpusEncoder creates a pure-Go Opus encoder through the compatibility API.
//
// In addition to the upstream 48 kHz/20 ms CELT API, it supports the xiaozhi
// wire format of 8/12/16/24/48 kHz PCM, one or two channels, and 60 ms packets. That path
// resamples each 20 ms segment to 48 kHz and packs three CBR CELT frames into a
// single Opus Code 3 packet.
func CreateOpusEncoder(config *OpusEncoderConfig) (*OpusEncoder, error) {
	if config == nil {
		config = &OpusEncoderConfig{}
	}

	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = celtSampleRate
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
		frameDuration = compatFrame20ms
	}

	opts := []EncoderOption{
		WithChannels(channels),
		WithApplication(application),
	}
	if config.Bitrate != 0 {
		opts = append(opts, WithBitrate(config.Bitrate))
	}

	if frameDuration == compatFrame60ms {
		bandwidth, err := compatCELTBandwidth(sampleRate)
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			WithSampleRate(celtSampleRate),
			WithVBR(false),
			WithBandwidth(bandwidth),
		)
		encoder, err := NewEncoder(opts...)
		if err != nil {
			return nil, err
		}

		result := &OpusEncoder{
			encoder:          encoder,
			channels:         channels,
			bridge60ms:       true,
			sourceSampleRate: sampleRate,
		}
		if sampleRate != celtSampleRate {
			for ch := 0; ch < channels; ch++ {
				if err = result.resamplers[ch].Init(sampleRate, celtSampleRate); err != nil {
					return nil, fmt.Errorf("initialize %d Hz resampler: %w", sampleRate, err)
				}
			}
		}

		return result, nil
	}

	if sampleRate != celtSampleRate || frameDuration != compatFrame20ms {
		return nil, fmt.Errorf(
			"%w: sample rate %d Hz with frame duration %s",
			errUnsupportedConfigurationMode,
			sampleRate,
			frameDuration,
		)
	}
	opts = append(opts, WithSampleRate(sampleRate))
	encoder, err := NewEncoder(opts...)
	if err != nil {
		return nil, err
	}

	return &OpusEncoder{encoder: encoder, channels: channels}, nil
}

// Encode encodes S16LE PCM into an Opus packet.
func (e *OpusEncoder) Encode(in, out []byte) (int, error) {
	if e.bridge60ms {
		return e.encode60ms(in, out)
	}

	return e.encoder.Encode(in, out)
}

// encode60ms resamples each 20 ms source segment to 48 kHz through the SILK
// resampler and packs three CELT frames into one Opus Code 3 CBR packet.
// Resampler state persists across packets, so packet boundaries are continuous.
func (e *OpusEncoder) encode60ms(in, out []byte) (int, error) {
	sourceSamples := e.sourceSampleRate / 50
	expectedBytes := sourceSamples * xiaozhiSubframeCount * e.channels * 2
	if len(in) != expectedBytes {
		return 0, fmt.Errorf("%w: got %d bytes, want %d", errInvalidFrameSize, len(in), expectedBytes)
	}

	payloadBytes := e.encoder.frameBytes()
	packetBytes := 2 + xiaozhiSubframeCount*payloadBytes
	if len(out) < packetBytes {
		return 0, errOutBufferTooSmall
	}

	writeOffset := 2
	for frame := 0; frame < xiaozhiSubframeCount; frame++ {
		for ch := 0; ch < e.channels; ch++ {
			for sample := 0; sample < sourceSamples; sample++ {
				offset := ((frame*sourceSamples+sample)*e.channels + ch) * 2
				e.resampleInput[ch][sample] = float32(int16(binary.LittleEndian.Uint16(in[offset:]))) / 32768
			}
			if e.sourceSampleRate == celtSampleRate {
				copy(e.resampleOutput[ch][:], e.resampleInput[ch][:sourceSamples])
			} else if err := e.resamplers[ch].Resample(
				e.resampleInput[ch][:sourceSamples],
				e.resampleOutput[ch][:],
			); err != nil {
				return 0, fmt.Errorf("resample %d Hz PCM: %w", e.sourceSampleRate, err)
			}
		}

		for sample := 0; sample < xiaozhiOutputSamples20ms; sample++ {
			for ch := 0; ch < e.channels; ch++ {
				e.interleavedPCM48k[sample*e.channels+ch] = e.resampleOutput[ch][sample]
			}
		}

		n, err := e.encoder.EncodeFloat32(
			e.interleavedPCM48k[:xiaozhiOutputSamples20ms*e.channels],
			e.framePacketScratch[:],
		)
		if err != nil {
			return 0, err
		}
		if n != payloadBytes+1 {
			return 0, fmt.Errorf("%w: got %d-byte frame, want %d", errInvalidFrameByteBudget, n, payloadBytes+1)
		}
		if frame == 0 {
			out[0] = (e.framePacketScratch[0] &^ byte(0x03)) | byte(frameCodeArbitraryFrames)
			out[1] = xiaozhiSubframeCount
		}
		copy(out[writeOffset:], e.framePacketScratch[1:n])
		writeOffset += payloadBytes
	}

	return packetBytes, nil
}

func compatCELTBandwidth(sampleRate int) (Bandwidth, error) {
	switch sampleRate {
	case 8000:
		return BandwidthNarrowband, nil
	case 12000, 16000:
		return BandwidthWideband, nil
	case 24000:
		return BandwidthSuperwideband, nil
	case celtSampleRate:
		return BandwidthFullband, nil
	default:
		return 0, fmt.Errorf("%w: %d", errInvalidSampleRate, sampleRate)
	}
}

// Close releases encoder resources. It is a no-op for the pure-Go encoder.
func (e *OpusEncoder) Close() error {
	return nil
}

// CreateOpusDecoder creates a pure-Go Opus decoder through the compatibility API.
func CreateOpusDecoder(config *OpusDecoderConfig) (*OpusDecoder, error) {
	if config == nil {
		config = &OpusDecoderConfig{}
	}

	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = celtSampleRate
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

// Decode decodes an Opus packet into S16LE PCM.
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

// Close releases decoder resources. It is a no-op for the pure-Go decoder.
func (d *OpusDecoder) Close() error {
	return nil
}
