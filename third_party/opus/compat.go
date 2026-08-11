// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package opus

import "time"

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
	encoder *Encoder
}

// OpusDecoder wraps Decoder with a qrtc-compatible method surface.
type OpusDecoder struct {
	decoder  Decoder
	channels int
}

// CreateOpusEncoder creates a pure-Go Opus encoder through the compatibility API.
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

// Encode encodes S16LE PCM into an Opus packet.
func (e *OpusEncoder) Encode(in, out []byte) (int, error) {
	return e.encoder.Encode(in, out)
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
