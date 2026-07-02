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
