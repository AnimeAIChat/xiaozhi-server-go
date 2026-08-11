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
	require.Positive(t, n)

	out := make([]byte, xiaozhiFrameSamples24k*2)
	decoded, err := decoder.Decode(packet[:n], out)
	require.NoError(t, err)
	assert.Equal(t, xiaozhiFrameSamples24k*2, decoded)
}
