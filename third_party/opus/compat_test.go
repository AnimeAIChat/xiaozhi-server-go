// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

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
	require.Greater(t, n, 2)

	toc := tableOfContentsHeader(packet[0])
	assert.Equal(t, configurationModeCELTOnly, toc.configuration().mode())
	assert.Equal(t, BandwidthSuperwideband, toc.configuration().bandwidth())
	assert.Equal(t, frameCodeArbitraryFrames, toc.frameCode())
	isVBR, hasPadding, frameCount := parseFrameCountByte(packet[1])
	assert.False(t, isVBR)
	assert.False(t, hasPadding)
	assert.Equal(t, byte(xiaozhiSubframeCount), frameCount)

	out := make([]byte, xiaozhiFrameSamples24k*2)
	decoded, err := decoder.Decode(packet[:n], out)
	require.NoError(t, err)
	assert.Equal(t, xiaozhiFrameSamples24k*2, decoded)
}

func TestCompatibilityAPIRejectsUnsupportedXiaozhiFormat(t *testing.T) {
	_, err := CreateOpusEncoder(&OpusEncoderConfig{
		SampleRate:    24000,
		MaxChannels:   1,
		FrameDuration: 20 * time.Millisecond,
	})
	assert.ErrorIs(t, err, errUnsupportedConfigurationMode)
}
