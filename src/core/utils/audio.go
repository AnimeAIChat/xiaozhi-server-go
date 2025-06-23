package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hajimehoshi/go-mp3"
	opus "github.com/qrtc/opus-go"
)

// OpusDecoder 封装opus解码器
type OpusDecoder struct {
	decoder   *opus.OpusDecoder
	mu        sync.Mutex
	config    *OpusDecoderConfig
	outBuffer []byte
}

// OpusDecoderConfig 解码器配置
type OpusDecoderConfig struct {
	SampleRate  int
	MaxChannels int
}

// NewOpusDecoder 创建新的opus解码器
func NewOpusDecoder(config *OpusDecoderConfig) (*OpusDecoder, error) {
	if config == nil {
		config = &OpusDecoderConfig{
			SampleRate:  24000, // 默认使用24kHz采样率
			MaxChannels: 1,     // 默认单通道
		}
	}

	libConfig := &opus.OpusDecoderConfig{
		SampleRate:  config.SampleRate,
		MaxChannels: config.MaxChannels,
	}

	decoder, err := opus.CreateOpusDecoder(libConfig)
	if err != nil {
		return nil, fmt.Errorf("创建Opus解码器失败: %v", err)
	}

	bufSize := config.SampleRate * 2 * config.MaxChannels * 120 / 1000
	if bufSize < 8192 {
		bufSize = 8192 // 至少8KB的缓冲区
	}

	return &OpusDecoder{
		decoder:   decoder,
		config:    config,
		outBuffer: make([]byte, bufSize),
	}, nil
}

// Decode 解码opus数据为PCM
func (d *OpusDecoder) Decode(opusData []byte) ([]byte, error) {
	if len(opusData) == 0 {
		return nil, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// 使用预分配的缓冲区
	n, err := d.decoder.Decode(opusData, d.outBuffer)
	if err != nil {
		return nil, fmt.Errorf("Opus解码失败: %v", err)
	}

	// 返回解码后的PCM数据的副本
	result := make([]byte, n)
	copy(result, d.outBuffer[:n])
	return result, nil
}

// Close 关闭解码器
func (d *OpusDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.decoder != nil {
		if err := d.decoder.Close(); err != nil {
			return fmt.Errorf("关闭Opus解码器失败: %v", err)
		}
		d.decoder = nil
	}
	return nil
}

func MP3ToPCMData(audioFile string) ([][]byte, error) {
	file, err := os.Open(audioFile)
	if err != nil {
		return nil, fmt.Errorf("打开音频文件失败: %v", err)
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return nil, fmt.Errorf("创建MP3解码器失败: %v", err)
	}

	mp3SampleRate := decoder.SampleRate()

	// 检查采样率是否支持
	supportedRates := map[int]bool{8000: true, 12000: true, 16000: true, 24000: true, 48000: true}
	if !supportedRates[mp3SampleRate] {
		return nil, fmt.Errorf("MP3采样率 %dHz 不被Opus直接支持，需要重采样", mp3SampleRate)
	}

	// decoder.Length() 返回解码后的PCM数据总字节数 (16-bit little-endian stereo)
	pcmBytes := make([]byte, decoder.Length())
	// ReadFull确保读取所有请求的字节，否则返回错误
	if _, err := io.ReadFull(decoder, pcmBytes); err != nil {
		// 如果 decoder.Length() 为 0, pcmBytes 为空, ReadFull 读取 0 字节, 返回 nil 错误，这是正常的。
		// 如果 decoder.Length() > 0 且 ReadFull 返回错误, 表示未能读取完整的PCM数据。
		return nil, fmt.Errorf("读取PCM数据失败: %v", err)
	}

	// go-mp3 解码为 16-bit little-endian stereo PCM.
	// pcmBytes 包含交错的立体声数据 (LRLRLR...).
	// 每个立体声样本对 (左16位, 右16位) 占用4字节.
	// numMonoSamples 是转换后得到的16位单声道样本的数量.
	numMonoSamples := len(pcmBytes) / 4

	if numMonoSamples == 0 {
		// 处理 pcmBytes 为空或数据不足以形成一个单声道样本的情况 (即少于4字节).
		return [][]byte{}, nil // 返回空数据
	}

	pcmMonoInt16 := make([]int16, numMonoSamples)
	for i := 0; i < numMonoSamples; i++ {
		// 从pcmBytes中提取16位小端序的左右声道样本
		// pcmBytes[i*4+0] = 左声道低字节, pcmBytes[i*4+1] = 左声道高字节
		// pcmBytes[i*4+2] = 右声道低字节, pcmBytes[i*4+3] = 右声道高字节
		leftSample := int16(uint16(pcmBytes[i*4+0]) | (uint16(pcmBytes[i*4+1]) << 8))
		rightSample := int16(uint16(pcmBytes[i*4+2]) | (uint16(pcmBytes[i*4+3]) << 8))

		// 通过平均值混合为单声道样本
		// 使用int32进行中间求和以防止在除法前溢出
		pcmMonoInt16[i] = int16((int32(leftSample) + int32(rightSample)) / 2)
	}

	// 将 []int16 类型的单声道PCM数据转换为 []byte (仍然是16位小端序)
	monoPcmDataBytes := make([]byte, numMonoSamples*2) // 每个int16样本占用2字节
	for i, sample := range pcmMonoInt16 {
		monoPcmDataBytes[i*2] = byte(sample)        // 低字节 (LSB)
		monoPcmDataBytes[i*2+1] = byte(sample >> 8) // 高字节 (MSB)
	}

	// 函数签名要求返回 [][]byte.
	// 将整个单声道PCM数据作为外部切片中的单个段/切片返回.
	return [][]byte{monoPcmDataBytes}, nil
}

func SaveAudioToWavFile(data []byte, fileName string, sampleRate int, channels int, bitsPerSample int) error {
	if fileName == "" {
		fileName = "output.wav"
	}

	isNewFile := false
	fileInfo, err := os.Stat(fileName)

	// 检查文件是否存在
	if os.IsNotExist(err) {
		isNewFile = true
	}

	var file *os.File
	if isNewFile {
		// 创建新文件
		file, err = os.Create(fileName)
		if err != nil {
			return fmt.Errorf("创建文件失败: %v", err)
		}
		defer file.Close()

		// 写入WAV文件头
		if err := writeWavHeader(file, 0, sampleRate, channels, bitsPerSample); err != nil {
			return fmt.Errorf("写入WAV头失败: %v", err)
		}
	}

	// 打开现有文件进行追加
	file, err = os.OpenFile(fileName, os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 获取当前数据大小
	var currentDataSize int64
	if !isNewFile {
		currentDataSize = fileInfo.Size() - 44 // 减去WAV头大小(44字节)
	}

	// 在文件末尾追加新数据
	_, err = file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("定位文件末尾失败: %v", err)
	}

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("写入数据失败: %v", err)
	}

	// 更新WAV头中的数据大小
	newDataSize := currentDataSize + int64(len(data))
	file.Seek(0, io.SeekStart)
	if err := writeWavHeader(file, int(newDataSize), sampleRate, channels, bitsPerSample); err != nil {
		return fmt.Errorf("更新WAV头失败: %v", err)
	}

	return nil
}

// 写入WAV文件头
func writeWavHeader(file *os.File, dataSize int, sampleRate, channels, bitsPerSample int) error {
	// RIFF块
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))

	// 文件总长度 = 数据大小 + 头部大小(36) - 8
	fileSize := uint32(dataSize + 36)
	header[4] = byte(fileSize)
	header[5] = byte(fileSize >> 8)
	header[6] = byte(fileSize >> 16)
	header[7] = byte(fileSize >> 24)

	// 文件类型
	copy(header[8:12], []byte("WAVE"))

	// 格式块
	copy(header[12:16], []byte("fmt "))

	// 格式块大小(16字节)
	header[16] = 16
	header[17] = 0
	header[18] = 0
	header[19] = 0

	// 音频格式(1表示PCM)
	header[20] = 1
	header[21] = 0

	// 通道数
	header[22] = byte(channels)
	header[23] = 0

	// 采样率
	header[24] = byte(sampleRate)
	header[25] = byte(sampleRate >> 8)
	header[26] = byte(sampleRate >> 16)
	header[27] = byte(sampleRate >> 24)

	// 字节率 = 采样率 × 通道数 × 位深度/8
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	header[28] = byte(byteRate)
	header[29] = byte(byteRate >> 8)
	header[30] = byte(byteRate >> 16)
	header[31] = byte(byteRate >> 24)

	// 块对齐 = 通道数 × 位深度/8
	blockAlign := uint16(channels * bitsPerSample / 8)
	header[32] = byte(blockAlign)
	header[33] = byte(blockAlign >> 8)

	// 位深度
	header[34] = byte(bitsPerSample)
	header[35] = byte(bitsPerSample >> 8)

	// 数据块
	copy(header[36:40], []byte("data"))

	// 数据大小
	header[40] = byte(dataSize)
	header[41] = byte(dataSize >> 8)
	header[42] = byte(dataSize >> 16)
	header[43] = byte(dataSize >> 24)

	_, err := file.Write(header)
	return err
}

// 保留原来的函数，但使用新函数
func SaveAudioToFile(data []byte) error {
	// 默认使用16kHz, 单声道, 16位
	return SaveAudioToWavFile(data, "output.wav", 16000, 1, 16)
}

func ReadPCMDataFromWavFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开WAV文件失败: %v", err)
	}
	defer file.Close()

	// 跳过WAV头
	header := make([]byte, 44)
	if _, err := file.Read(header); err != nil {
		return nil, fmt.Errorf("读取WAV头失败: %v", err)
	}

	// 读取PCM数据
	pcmData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取PCM数据失败: %v", err)
	}

	return pcmData, nil
}

func AudioToPCMData(audioFile string) ([][]byte, float64, error) {
	file, err := os.Open(audioFile)
	if err != nil {
		return nil, 0, fmt.Errorf("打开音频文件失败: %v", err)
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return nil, 0, fmt.Errorf("创建MP3解码器失败: %v", err)
	}

	mp3SampleRate := decoder.SampleRate()
	//fmt.Println("AudioToPCMData 原始MP3采样率:", mp3SampleRate)
	// 目标采样率设为24kHz
	targetSampleRate := 24000

	// decoder.Length() 返回解码后的PCM数据总字节数 (16-bit little-endian stereo)
	pcmBytes := make([]byte, decoder.Length())
	// ReadFull确保读取所有请求的字节，否则返回错误
	if _, err := io.ReadFull(decoder, pcmBytes); err != nil {
		// 如果 decoder.Length() 为 0, pcmBytes 为空, ReadFull 读取 0 字节, 返回 nil 错误，这是正常的。
		// 如果 decoder.Length() > 0 且 ReadFull 返回错误, 表示未能读取完整的PCM数据。
		return nil, 0, fmt.Errorf("读取PCM数据失败: %v", err)
	}

	// go-mp3 解码为 16-bit little-endian stereo PCM.
	// pcmBytes 包含交错的立体声数据 (LRLRLR...).
	// 每个立体声样本对 (左16位, 右16位) 占用4字节.
	// numMonoSamples 是转换后得到的16位单声道样本的数量.
	numMonoSamples := len(pcmBytes) / 4

	if numMonoSamples == 0 {
		// 处理 pcmBytes 为空或数据不足以形成一个单声道样本的情况 (即少于4字节).
		return [][]byte{}, 0, nil // 返回空数据
	}

	pcmMonoInt16 := make([]int16, numMonoSamples)
	for i := 0; i < numMonoSamples; i++ {
		// 从pcmBytes中提取16位小端序的左右声道样本
		// pcmBytes[i*4+0] = 左声道低字节, pcmBytes[i*4+1] = 左声道高字节
		// pcmBytes[i*4+2] = 右声道低字节, pcmBytes[i*4+3] = 右声道高字节
		leftSample := int16(uint16(pcmBytes[i*4+0]) | (uint16(pcmBytes[i*4+1]) << 8))
		rightSample := int16(uint16(pcmBytes[i*4+2]) | (uint16(pcmBytes[i*4+3]) << 8))

		// 通过平均值混合为单声道样本
		// 使用int32进行中间求和以防止在除法前溢出
		pcmMonoInt16[i] = int16((int32(leftSample) + int32(rightSample)) / 2)
	}

	// 重采样到目标采样率（如果需要）
	var resampledPcmInt16 []int16
	var finalSampleRate int

	if mp3SampleRate != targetSampleRate {
		fmt.Printf("重采样从 %dHz 到 %dHz\n", mp3SampleRate, targetSampleRate)
		resampledPcmInt16 = resamplePCM(pcmMonoInt16, mp3SampleRate, targetSampleRate)
		finalSampleRate = targetSampleRate
	} else {
		resampledPcmInt16 = pcmMonoInt16
		finalSampleRate = mp3SampleRate
	}

	// 将 []int16 类型的单声道PCM数据转换为 []byte (仍然是16位小端序)
	monoPcmDataBytes := make([]byte, len(resampledPcmInt16)*2) // 每个int16样本占用2字节
	for i, sample := range resampledPcmInt16 {
		monoPcmDataBytes[i*2] = byte(sample)        // 低字节 (LSB)
		monoPcmDataBytes[i*2+1] = byte(sample >> 8) // 高字节 (MSB)
	}

	//音频播放时长（基于重采样后的数据）
	duration := float64(len(resampledPcmInt16)) / float64(finalSampleRate) // 单声道PCM数据的时长 (秒)

	// 函数签名要求返回 [][]byte.
	// 将整个单声道PCM数据作为外部切片中的单个段/切片返回.
	return [][]byte{monoPcmDataBytes}, duration, nil
}

// AudioToOpusData 将音频文件转换为Opus数据块
func AudioToOpusData(audioFile string) ([][]byte, float64, error) {

	var pcmData [][]byte
	var err error
	var duration float64

	// 获取采样率 (固定使用24000Hz作为Opus编码的采样率)
	// 如果采样率不是24000Hz，PCMSlicesToOpusData会处理重采样
	opusSampleRate := 24000
	channels := 1

	if strings.HasSuffix(audioFile, ".mp3") {
		// 先将MP3转为PCM
		pcmData, duration, err = AudioToPCMData(audioFile)
		if err != nil {
			return nil, 0, fmt.Errorf("PCM转换失败: %v", err)
		}

		if len(pcmData) == 0 {
			return nil, 0, fmt.Errorf("PCM转换结果为空")
		}

	} else {
		var singlePcmData []byte
		singlePcmData, _ = ReadPCMDataFromWavFile(audioFile)
		pcmData = [][]byte{singlePcmData}
	}

	// 将PCM转换为Opus
	opusData, err := PCMSlicesToOpusData(pcmData, opusSampleRate, channels, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("PCM转Opus失败: %v", err)
	}

	return opusData, duration, nil
}

// CopyAudioFile 复制音频文件
func CopyAudioFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// SaveAudioFile 保存音频数据到文件
func SaveAudioFile(data []byte, filename string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("写入音频数据失败: %v", err)
	}

	return nil
}

// PCMToOpusData 将PCM数据编码为Opus格式
func PCMToOpusData(pcmData []byte, sampleRate int, channels int) ([]byte, error) {
	if len(pcmData) == 0 {
		return nil, fmt.Errorf("PCM数据为空")
	}

	// 检查采样率是否支持
	supportedRates := map[int]bool{8000: true, 12000: true, 16000: true, 24000: true, 48000: true}
	if !supportedRates[sampleRate] {
		return nil, fmt.Errorf("采样率 %dHz 不被Opus支持，仅支持8000/12000/16000/24000/48000Hz", sampleRate)
	}

	// 确保PCM数据长度是偶数，这是16位PCM所必需的
	if len(pcmData)%2 != 0 {
		return nil, fmt.Errorf("PCM数据长度必须是偶数（16位采样）")
	}

	// 将PCM字节转换为int16样本
	numSamples := len(pcmData) / 2 / channels
	pcmInt16 := make([]int16, numSamples*channels)
	for i := 0; i < numSamples*channels; i++ {
		// 读取小端序的16位样本
		pcmInt16[i] = int16(uint16(pcmData[i*2]) | (uint16(pcmData[i*2+1]) << 8))
	}

	// 计算每帧包含的样本数（60ms帧）
	samplesPerFrame := (sampleRate * 60) / 1000                         // 60ms帧
	framesCount := (numSamples + samplesPerFrame - 1) / samplesPerFrame // 向上取整

	// 根据帧大小调整样本数组的大小
	paddedSampleCount := framesCount * samplesPerFrame
	if paddedSampleCount > numSamples {
		// 扩展样本数组到帧边界
		paddedSamples := make([]int16, paddedSampleCount*channels)
		copy(paddedSamples, pcmInt16)
		pcmInt16 = paddedSamples
	}

	// 将int16样本转回为字节数组
	adjustedPcmData := make([]byte, len(pcmInt16)*2)
	for i, sample := range pcmInt16 {
		adjustedPcmData[i*2] = byte(sample)        // 低字节
		adjustedPcmData[i*2+1] = byte(sample >> 8) // 高字节
	}

	// 创建Opus编码器
	encoder, err := opus.CreateOpusEncoder(&opus.OpusEncoderConfig{
		SampleRate:    sampleRate,
		MaxChannels:   channels,
		Application:   opus.AppVoIP,
		FrameDuration: opus.Framesize60Ms, // 使用60ms帧长
	})
	if err != nil {
		return nil, fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	defer encoder.Close()

	// 输出缓冲区
	outBuf := make([]byte, 4096)

	// 编码PCM数据到Opus
	n, err := encoder.Encode(adjustedPcmData, outBuf)
	if err != nil {
		return nil, fmt.Errorf("Opus编码失败: %v", err)
	}

	// 返回实际编码的数据
	return outBuf[:n], nil
}

// PCMToOpusFile 将PCM数据编码为Opus并保存到文件
func PCMToOpusFile(pcmData []byte, filename string, sampleRate int, channels int) error {
	opusData, err := PCMToOpusData(pcmData, sampleRate, channels)
	if err != nil {
		return err
	}

	return SaveAudioFile(opusData, filename)
}

// MP3ToOpusData 将MP3文件转换为Opus格式
func MP3ToOpusData(audioFile string) ([]byte, error) {
	// 先将MP3转为PCM
	pcmDataSlices, err := MP3ToPCMData(audioFile)
	if err != nil {
		return nil, fmt.Errorf("MP3转PCM失败: %v", err)
	}

	if len(pcmDataSlices) == 0 || len(pcmDataSlices[0]) == 0 {
		return nil, fmt.Errorf("MP3解码后PCM数据为空")
	}

	// 打开MP3文件获取采样率
	file, err := os.Open(audioFile)
	if err != nil {
		return nil, fmt.Errorf("打开MP3文件失败: %v", err)
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return nil, fmt.Errorf("创建MP3解码器失败: %v", err)
	}

	// 获取采样率
	sampleRate := decoder.SampleRate()
	fmt.Println("MP3采样率:", sampleRate)

	// 确保PCM数据长度是偶数
	pcmData := pcmDataSlices[0]
	if len(pcmData)%2 != 0 {
		return nil, fmt.Errorf("PCM数据长度必须是偶数（16位采样）")
	}

	// 将PCM字节转换为int16样本
	numSamples := len(pcmData) / 2 // 单通道
	pcmInt16 := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		// 读取小端序的16位样本
		pcmInt16[i] = int16(uint16(pcmData[i*2]) | (uint16(pcmData[i*2+1]) << 8))
	}

	// 计算每帧包含的样本数（60ms帧）
	samplesPerFrame := (sampleRate * 60) / 1000                         // 60ms帧
	framesCount := (numSamples + samplesPerFrame - 1) / samplesPerFrame // 向上取整

	// 根据帧大小调整样本数组的大小
	paddedSampleCount := framesCount * samplesPerFrame
	if paddedSampleCount > numSamples {
		// 扩展样本数组到帧边界
		paddedSamples := make([]int16, paddedSampleCount)
		copy(paddedSamples, pcmInt16)
		pcmInt16 = paddedSamples
	}

	// 将int16样本转回为字节数组
	adjustedPcmData := make([]byte, len(pcmInt16)*2)
	for i, sample := range pcmInt16 {
		adjustedPcmData[i*2] = byte(sample)        // 低字节
		adjustedPcmData[i*2+1] = byte(sample >> 8) // 高字节
	}

	// 创建Opus编码器
	encoder, err := opus.CreateOpusEncoder(&opus.OpusEncoderConfig{
		SampleRate:    sampleRate,
		MaxChannels:   1, // 单声道
		Application:   opus.AppVoIP,
		FrameDuration: opus.Framesize60Ms, // 使用60ms帧长
	})
	if err != nil {
		return nil, fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	defer encoder.Close()

	// 输出缓冲区
	outBuf := make([]byte, 4096)

	// 编码PCM数据到Opus
	n, err := encoder.Encode(adjustedPcmData, outBuf)
	if err != nil {
		return nil, fmt.Errorf("Opus编码失败: %v", err)
	}

	// 返回实际编码的数据
	return outBuf[:n], nil
}

// MP3ToOpusFile 将MP3文件转换为Opus并保存到文件
func MP3ToOpusFile(inputFile, outputFile string, bitrate int) error {
	opusData, err := MP3ToOpusData(inputFile)
	if err != nil {
		return err
	}

	return SaveAudioFile(opusData, outputFile)
}

// PCMSlicesToOpusData 将PCM数据切片批量编码为Opus格式
func PCMSlicesToOpusData(pcmSlices [][]byte, sampleRate int, channels int, bitrate int) ([][]byte, error) {
	if len(pcmSlices) == 0 {
		return nil, fmt.Errorf("PCM数据切片为空")
	}

	// 检查采样率是否支持
	supportedRates := map[int]bool{8000: true, 12000: true, 16000: true, 24000: true, 48000: true}
	if !supportedRates[sampleRate] {
		return nil, fmt.Errorf("采样率 %dHz 不被Opus支持，仅支持8000/12000/16000/24000/48000Hz", sampleRate)
	}

	// 创建Opus编码器
	encoder, err := opus.CreateOpusEncoder(&opus.OpusEncoderConfig{
		SampleRate:    sampleRate,
		MaxChannels:   channels,
		Application:   opus.AppVoIP,
		FrameDuration: opus.Framesize60Ms, // 使用60ms帧长
	})
	if err != nil {
		return nil, fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	defer encoder.Close()

	// 所有编码后的Opus数据包
	var allOpusPackets [][]byte

	// 计算每帧样本数 (60ms帧)
	samplesPerFrame := (sampleRate * 60) / 1000 // 60ms帧
	// 每个样本的字节数 (16位 = 2字节)
	bytesPerSample := 2 * channels
	// 每帧字节数
	bytesPerFrame := samplesPerFrame * bytesPerSample

	for _, pcmSlice := range pcmSlices {
		if len(pcmSlice) == 0 {
			continue
		}

		// 确保PCM数据长度是偶数
		if len(pcmSlice)%2 != 0 {
			pcmSlice = pcmSlice[:len(pcmSlice)-1] // 截断最后一个字节
			if len(pcmSlice) == 0 {
				continue
			}
		}

		// 计算这个PCM片段可以分成多少帧
		numFrames := len(pcmSlice) / bytesPerFrame
		if len(pcmSlice)%bytesPerFrame != 0 {
			numFrames++ // 如果有剩余数据，额外增加一帧
		}

		// 逐帧处理PCM数据
		for frameIdx := 0; frameIdx < numFrames; frameIdx++ {
			frameStart := frameIdx * bytesPerFrame
			frameEnd := frameStart + bytesPerFrame

			// 确保不越界
			if frameEnd > len(pcmSlice) {
				frameEnd = len(pcmSlice)
			}

			// 当前帧的PCM数据
			framePcm := pcmSlice[frameStart:frameEnd]

			// 如果最后一帧数据不足，需要填充静音数据到完整帧大小
			if len(framePcm) < bytesPerFrame {
				paddedFrame := make([]byte, bytesPerFrame)
				copy(paddedFrame, framePcm)
				framePcm = paddedFrame
			}

			// 分配输出缓冲区 (Opus编码后的数据通常比PCM小)
			outBuf := make([]byte, len(framePcm))

			// 编码这一帧PCM数据到Opus
			n, err := encoder.Encode(framePcm, outBuf)
			if err != nil {
				continue // 跳过这一帧，继续处理下一帧
			}

			if n == 0 {
				continue // 跳过空帧
			}

			// 将编码后的Opus数据添加到结果集
			allOpusPackets = append(allOpusPackets, outBuf[:n])
		}
	}

	if len(allOpusPackets) == 0 {
		return nil, fmt.Errorf("所有PCM切片编码后为空")
	}

	return allOpusPackets, nil
}

// resamplePCM 使用线性插值对PCM数据进行重采样
func resamplePCM(input []int16, inputSampleRate, outputSampleRate int) []int16 {
	if inputSampleRate == outputSampleRate {
		return input
	}

	inputLength := len(input)
	if inputLength == 0 {
		return []int16{}
	}

	// 计算重采样比率
	ratio := float64(inputSampleRate) / float64(outputSampleRate)
	outputLength := int(float64(inputLength) / ratio)

	if outputLength == 0 {
		return []int16{}
	}

	output := make([]int16, outputLength)

	for i := 0; i < outputLength; i++ {
		// 计算在输入数组中的位置
		srcIndex := float64(i) * ratio

		// 获取整数和小数部分
		index := int(srcIndex)
		fraction := srcIndex - float64(index)

		if index >= inputLength-1 {
			// 如果超出边界，使用最后一个样本
			output[i] = input[inputLength-1]
		} else {
			// 线性插值
			sample1 := float64(input[index])
			sample2 := float64(input[index+1])
			interpolated := sample1 + fraction*(sample2-sample1)
			output[i] = int16(interpolated)
		}
	}

	return output
}
