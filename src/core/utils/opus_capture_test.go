package utils

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"testing"

	"github.com/AnimeAIChat/opus/pkg/oggreader"
)

func TestSaveOpusPacketsWritesReadableOggOpus(t *testing.T) {
	packets := validOpusCaptureTestPackets(t)
	path, err := SaveOpusPackets(t.TempDir(), "device:one", "session/one", 24000, 1, 60, packets)
	if err != nil {
		t.Fatalf("SaveOpusPackets() error = %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, header, err := oggreader.NewWith(file)
	if err != nil {
		t.Fatalf("oggreader.NewWith() error = %v", err)
	}
	if header.SampleRate != 24000 || header.Channels != 1 {
		t.Fatalf("Ogg header = %dHz/%d channels, want 24000Hz/1", header.SampleRate, header.Channels)
	}
	tags, _, err := reader.ParseNextPacket()
	if err != nil {
		t.Fatalf("ParseNextPacket(OpusTags) error = %v", err)
	}
	if !bytes.HasPrefix(tags, []byte("OpusTags")) {
		t.Fatalf("first metadata packet = %x, want OpusTags", tags)
	}
	for i, want := range packets {
		got, _, err := reader.ParseNextPacket()
		if err != nil {
			t.Fatalf("ParseNextPacket(%d) error = %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("packet %d = %x, want %x", i, got, want)
		}
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 FFmpeg，跳过独立解码验证")
	}
	if output, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("FFmpeg/libopus 独立解码失败: %v\n%s", err, output)
	}
}

func validOpusCaptureTestPackets(t *testing.T) [][]byte {
	t.Helper()
	const sampleRate = 24000
	const samplesPerFrame = sampleRate * 60 / 1000
	pcm := make([]byte, samplesPerFrame*2)
	for i := range samplesPerFrame {
		sample := int16(math.Round(math.Sin(2*math.Pi*440*float64(i)/sampleRate) * 16000))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	packets, err := PCMSlicesToOpusData([][]byte{pcm, pcm}, sampleRate, 1, 24000)
	if err != nil {
		t.Fatalf("PCMSlicesToOpusData() error = %v", err)
	}
	if len(packets) != 2 || len(packets[0]) == 0 {
		t.Fatalf("unexpected generated Opus packets: %d", len(packets))
	}
	return packets
}
