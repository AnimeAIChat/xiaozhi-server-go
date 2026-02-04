package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpusDecoder_DefaultConfig(t *testing.T) {
	decoder, err := NewOpusDecoder(nil)

	assert.NoError(t, err)
	assert.NotNil(t, decoder)
	assert.NotNil(t, decoder.decoder)

	err = decoder.Close()
	assert.NoError(t, err)
}

func TestNewOpusDecoder_CustomConfig(t *testing.T) {
	config := &OpusDecoderConfig{
		SampleRate:  16000,
		MaxChannels: 2,
	}

	decoder, err := NewOpusDecoder(config)

	assert.NoError(t, err)
	assert.NotNil(t, decoder)
	assert.NotNil(t, decoder.decoder)
	assert.Equal(t, 16000, decoder.config.SampleRate)
	assert.Equal(t, 2, decoder.config.MaxChannels)

	err = decoder.Close()
	assert.NoError(t, err)
}

func TestOpusDecoder_Close(t *testing.T) {
	decoder, err := NewOpusDecoder(nil)
	require.NoError(t, err)

	err = decoder.Close()
	assert.NoError(t, err)

	err = decoder.Close()
	assert.NoError(t, err)
}

func TestOpusDecoder_Decode_EmptyData(t *testing.T) {
	decoder, err := NewOpusDecoder(nil)
	require.NoError(t, err)
	defer decoder.Close()

	result, err := decoder.Decode([]byte{})

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestWriteWavHeader(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.wav")

	file, err := os.Create(filePath)
	require.NoError(t, err)
	defer file.Close()

	err = writeWavHeader(file, 1000, 24000, 1, 16)

	assert.NoError(t, err)

	fileInfo, err := file.Stat()
	require.NoError(t, err)
	assert.Equal(t, int64(44), fileInfo.Size())
}

func TestWriteWavHeader_Stereo(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_stereo.wav")

	file, err := os.Create(filePath)
	require.NoError(t, err)
	defer file.Close()

	err = writeWavHeader(file, 2000, 44100, 2, 16)

	assert.NoError(t, err)

	fileInfo, err := file.Stat()
	require.NoError(t, err)
	assert.Equal(t, int64(44), fileInfo.Size())
}

func TestCopyAudioFile_SourceNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "not_exist.wav")
	dstPath := filepath.Join(tmpDir, "dst.wav")

	err := CopyAudioFile(srcPath, dstPath)

	assert.Error(t, err)
}

func TestPCMToOpusData_EmptyData(t *testing.T) {
	result, err := PCMToOpusData([]byte{}, 24000, 1)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMToOpusData_UnsupportedSampleRate(t *testing.T) {
	pcmData := make([]byte, 100)

	result, err := PCMToOpusData(pcmData, 12345, 1)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMToOpusData_InvalidDataLength(t *testing.T) {
	pcmData := make([]byte, 3)

	result, err := PCMToOpusData(pcmData, 24000, 1)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMToOpusData_Success(t *testing.T) {
	pcmData := make([]byte, 240)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	result, err := PCMToOpusData(pcmData, 24000, 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result), 0)
}

func TestPCMToOpusData_Stereo(t *testing.T) {
	pcmData := make([]byte, 480)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	result, err := PCMToOpusData(pcmData, 24000, 2)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result), 0)
}

func TestPCMToOpusFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "output.opus")

	pcmData := make([]byte, 240)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	err := PCMToOpusFile(pcmData, filePath, 24000, 1)

	assert.NoError(t, err)

	_, err = os.Stat(filePath)
	assert.NoError(t, err)
}

func TestPCMSlicesToOpusData_EmptySlices(t *testing.T) {
	result, err := PCMSlicesToOpusData([][]byte{}, 24000, 1, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMSlicesToOpusData_UnsupportedSampleRate(t *testing.T) {
	pcmData := make([]byte, 240)
	result, err := PCMSlicesToOpusData([][]byte{pcmData}, 12345, 1, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMSlicesToOpusData_EmptySlice(t *testing.T) {
	result, err := PCMSlicesToOpusData([][]byte{{}, {}}, 24000, 1, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMSlicesToOpusData_Success(t *testing.T) {
	pcmData := make([]byte, 480)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	result, err := PCMSlicesToOpusData([][]byte{pcmData}, 24000, 1, 0)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result), 0)
}

func TestPCMSlicesToOpusData_MultipleSlices(t *testing.T) {
	pcmData1 := make([]byte, 240)
	pcmData2 := make([]byte, 240)

	result, err := PCMSlicesToOpusData([][]byte{pcmData1, pcmData2}, 24000, 1, 0)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result))
}

func TestResamplePCM_SameRate(t *testing.T) {
	input := []int16{100, 200, 300, 400, 500}

	result := resamplePCM(input, 24000, 24000)

	assert.Equal(t, input, result)
}

func TestResamplePCM_EmptyInput(t *testing.T) {
	result := resamplePCM([]int16{}, 24000, 48000)

	assert.Equal(t, []int16{}, result)
}

func TestResamplePCM_Upsample(t *testing.T) {
	input := []int16{0, 1000, 2000, 3000}

	result := resamplePCM(input, 16000, 32000)

	assert.NotNil(t, result)
	assert.Equal(t, 8, len(result))
}

func TestResamplePCM_Downsample(t *testing.T) {
	input := make([]int16, 100)
	for i := range input {
		input[i] = int16(i * 10)
	}

	result := resamplePCM(input, 48000, 16000)

	assert.NotNil(t, result)
	assert.Greater(t, len(result), 0)
}

func TestResamplePCM_SingleSample(t *testing.T) {
	input := []int16{100}

	result := resamplePCM(input, 24000, 48000)

	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, len(result), 1)
}

func TestReadPCMDataFromWavFile_NotExist(t *testing.T) {
	result, err := ReadPCMDataFromWavFile("/not/exist/file.wav")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPCMToOpusData_VariousSampleRates(t *testing.T) {
	testRates := []int{8000, 12000, 16000, 24000, 48000}

	for _, rate := range testRates {
		t.Run("", func(t *testing.T) {
			numSamples := rate / 100
			pcmData := make([]byte, numSamples*2)
			for i := range pcmData {
				pcmData[i] = byte(i % 256)
			}

			result, err := PCMToOpusData(pcmData, rate, 1)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Greater(t, len(result), 0)
		})
	}
}

func TestPCMToOpusData_SupportedSampleRates(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
	}{
		{"8000Hz", 8000},
		{"12000Hz", 12000},
		{"16000Hz", 16000},
		{"24000Hz", 24000},
		{"48000Hz", 48000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numSamples := tt.sampleRate / 100
			pcmData := make([]byte, numSamples*2)
			for i := range pcmData {
				pcmData[i] = byte(i % 256)
			}

			result, err := PCMToOpusData(pcmData, tt.sampleRate, 1)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Greater(t, len(result), 0)
		})
	}
}

func TestPCMToOpusData_UnsupportedRates(t *testing.T) {
	unsupportedRates := []int{6000, 22050, 44100, 96000}

	for _, rate := range unsupportedRates {
		t.Run("", func(t *testing.T) {
			pcmData := make([]byte, 240)

			result, err := PCMToOpusData(pcmData, rate, 1)

			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestPCMToOpusData_SingleChannel(t *testing.T) {
	pcmData := make([]byte, 480)

	result, err := PCMToOpusData(pcmData, 24000, 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result), 0)
}

func TestResamplePCM_VariousConversions(t *testing.T) {
	tests := []struct {
		name       string
		inputRate  int
		outputRate int
	}{
		{"HalfRate", 48000, 24000},
		{"DoubleRate", 16000, 32000},
		{"QuarterRate", 48000, 12000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]int16, 100)
			for i := range input {
				input[i] = int16(i * 10)
			}

			result := resamplePCM(input, tt.inputRate, tt.outputRate)

			assert.NotNil(t, result)
			assert.Greater(t, len(result), 0)
		})
	}
}
