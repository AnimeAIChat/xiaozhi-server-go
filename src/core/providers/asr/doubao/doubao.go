package doubao

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gorilla/websocket"
)

// Protocol constants
const (
	clientFullRequest   = 0x1
	clientAudioRequest  = 0x2
	serverFullResponse  = 0x9
	serverAck           = 0xB
	serverErrorResponse = 0xF
)

// Sequence types
const (
	noSequence  = 0x0
	negSequence = 0x2
)

// Serialization methods
const (
	noSerialization   = 0x0
	jsonFormat        = 0x1
	thriftFormat      = 0x3
	gzipCompression   = 0x1
	customCompression = 0xF

	// 超时设置
	idleTimeout = 10 * time.Second // 10秒没有新数据就结束识别
)

// Ensure Provider implements asr.Provider interface
var _ asr.Provider = (*Provider)(nil)

// Provider 豆包ASR提供者实现
type Provider struct {
	*asr.BaseProvider
	appID         string
	accessToken   string
	outputDir     string
	host          string
	wsURL         string
	chunkDuration int
	connectID     string
	logger        *utils.Logger // 添加日志记录器

	// 配置
	modelName     string
	endWindowSize int
	enablePunc    bool
	enableITN     bool
	enableDDC     bool

	// 流式识别相关字段
	conn        *websocket.Conn
	isStreaming bool
	reqID       string
	result      string
	err         error
	connMutex   sync.Mutex // 添加互斥锁保护连接状态
}

// requestPayload 请求数据结构
type requestPayload struct {
	App struct {
		AppID   string `json:"appid"`
		Cluster string `json:"cluster"`
		Token   string `json:"token"`
	} `json:"app"`
	User struct {
		UID string `json:"uid"`
	} `json:"user"`
	Request struct {
		ReqID          string `json:"reqid"`
		ShowUtterances bool   `json:"show_utterances"`
		Sequence       int    `json:"sequence"`
	} `json:"request"`
	Audio struct {
		Format   string `json:"format"`
		Rate     int    `json:"rate"`
		Language string `json:"language"`
		Bits     int    `json:"bits"`
		Channel  int    `json:"channel"`
		Codec    string `json:"codec"`
	} `json:"audio"`
}

// responsePayload 响应数据结构
type Utterance struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
	Definite  bool   `json:"definite"`
	// Words     []Word // Assuming Word struct might be needed later if word-level details are parsed
}

type AudioInfo struct {
	Duration int `json:"duration"`
}
type ResultPayload struct {
	Text       string                 `json:"text"`
	Utterances []Utterance            `json:"utterances,omitempty"`
	Additions  map[string]interface{} `json:"additions,omitempty"` // 新增 Additions 字段
}

type responsePayload struct {
	Code      int           `json:"code"`
	Result    ResultPayload `json:"result,omitempty"` // 修改为单个 ResultPayload
	AudioInfo AudioInfo     `json:"audio_info,omitempty"`
	Message   string        `json:"message,omitempty"` // For error messages
}

// NewProvider 创建豆包ASR提供者实例
func NewProvider(config *asr.Config, deleteFile bool) (*Provider, error) {
	base := asr.NewBaseProvider(config, deleteFile)

	// 从config.Data中获取配置
	appID, ok := config.Data["appid"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少appid配置")
	}

	accessToken, ok := config.Data["access_token"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少access_token配置")
	}

	// 确保输出目录存在
	outputDir, _ := config.Data["output_dir"].(string)
	if outputDir == "" {
		outputDir = "tmp/"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 创建连接ID
	connectID := fmt.Sprintf("%d", time.Now().UnixNano())

	// 创建一个简单的日志输出，不使用文件记录
	logger := &utils.Logger{}

	provider := &Provider{
		BaseProvider:  base,
		appID:         appID,
		accessToken:   accessToken,
		outputDir:     outputDir,
		host:          "openspeech.bytedance.com",
		wsURL:         "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream",
		chunkDuration: 200, // 固定使用200ms分片
		connectID:     connectID,
		logger:        logger, // 使用简单的logger

		// 默认配置
		modelName:     "bigmodel",
		endWindowSize: 800,
		enablePunc:    true,
		enableITN:     true,
		enableDDC:     false,
	}

	// 初始化音频处理
	provider.InitAudioProcessing()

	return provider, nil
}

// 读取根目录下的mp3文件，测试Transcribe方法
func (p *Provider) TestTranscribe() (string, error) {
	fmt.Println("TestTranscribe called")
	// 读取音频文件
	audioFile := "700.mp3" // 替换为实际的音频文件路径

	pcmData, err := utils.MP3ToPCMData(audioFile)
	if err != nil {
		fmt.Println("MP3转PCM失败: ", err.Error())
	}
	monoPcmDataBytes := []byte{}
	if len(pcmData) > 0 {
		monoPcmDataBytes = pcmData[0] // 提取第一个切片
		fmt.Printf("提取的单声道PCM数据长度: %d 字节\n", len(monoPcmDataBytes))

	} else {
		fmt.Println("没有PCM数据可提取")
	}

	result, err := p.Transcribe(context.Background(), monoPcmDataBytes)
	if err != nil {
		fmt.Println("转录失败: ", err.Error())
	} else {
		fmt.Print("result is ", result, "\n")
	}

	return result, nil
}

// Transcribe 实现asr.Provider接口的转录方法
func (p *Provider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if p.isStreaming {
		return "", fmt.Errorf("正在进行流式识别, 请先调用Reset")
	}

	// 创建临时文件
	tempFile := filepath.Join(p.outputDir, fmt.Sprintf("temp_%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(tempFile, audioData, 0644); err != nil {
		return "", fmt.Errorf("保存临时文件失败: %v", err)
	}
	defer func() {
		if p.DeleteFile() {
			os.Remove(tempFile)
		}
	}()

	// 初始化连接
	if err := p.Initialize(); err != nil {
		return "", err
	}
	defer p.Cleanup()

	// 添加音频数据
	if err := p.AddAudioWithContext(ctx, audioData); err != nil {
		return "", err
	}
	// 等待结果,无法立即返回正确的结果，通过回调函数返回
	return p.result, nil
}

// generateHeader 生成协议头
func (p *Provider) generateHeader(messageType uint8, flags uint8, serializationMethod uint8) []byte {
	header := make([]byte, 4)
	header[0] = (1 << 4) | 1                                 // 协议版本(4位) + 头大小(4位)
	header[1] = (messageType << 4) | flags                   // 消息类型(4位) + 消息标志(4位)
	header[2] = (serializationMethod << 4) | gzipCompression // 序列化方法(4位) + 压缩方法(4位)
	header[3] = 0                                            // 保留字段
	return header
}

// constructRequest 构造请求数据
func (p *Provider) constructRequest() map[string]interface{} {
	return map[string]interface{}{
		"user": map[string]interface{}{
			"uid": p.reqID,
		},
		"audio": map[string]interface{}{
			"format": "pcm",
			//"codec":    "opus", // 默认raw音频格式
			"rate":     16000,
			"bits":     16,
			"channel":  1,
			"language": "zh-CN", // Added language as per doc example
		},
		"request": map[string]interface{}{
			"model_name":      p.modelName,
			"end_window_size": p.endWindowSize,
			"enable_punc":     p.enablePunc,
			"enable_itn":      p.enableITN,
			"enable_ddc":      p.enableDDC,
			"result_type":     "single",
			"show_utterances": false, // Added show_utterances, default to false
		},
	}
}

// GetAudioBuffer 获取基类的audioBuffer
func (p *Provider) GetAudioBuffer() *bytes.Buffer {
	return p.BaseProvider.GetAudioBuffer()
}

// GetLastChunkTime 获取基类的lastChunkTime
func (p *Provider) GetLastChunkTime() time.Time {
	return p.BaseProvider.GetLastChunkTime()
}

// parseResponse 解析响应数据
func (p *Provider) parseResponse(data []byte) (map[string]interface{}, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("响应数据太短")
	}

	// 解析头部
	_ = data[0] >> 4 // protocol version
	headerSize := data[0] & 0x0f
	messageType := data[1] >> 4
	_ = data[1] & 0x0f // flags
	serializationMethod := data[2] >> 4
	compressionMethod := data[2] & 0x0f

	// 跳过头部获取payload
	payload := data[headerSize*4:]
	result := make(map[string]interface{})

	var payloadMsg []byte
	var payloadSize int32

	switch messageType {
	case serverFullResponse:
		// Doc: Header | Sequence | Payload size | Payload
		if len(payload) < 8 { // Need 4 bytes for sequence + 4 bytes for payload size
			return nil, fmt.Errorf("serverFullResponse payload too short for sequence and size: got %d bytes", len(payload))
		}
		seq := binary.BigEndian.Uint32(payload[0:4])
		result["seq"] = seq // Store WebSocket frame sequence
		payloadSize = int32(binary.BigEndian.Uint32(payload[4:8]))
		if len(payload) < 8+int(payloadSize) {
			return nil, fmt.Errorf("serverFullResponse payload too short for declared payload size: got %d bytes, expected header + %d bytes", len(payload), payloadSize)
		}
		payloadMsg = payload[8:]
	case serverAck:
		// Doc for serverAck is not detailed for ASR, but generally it might have a sequence
		if len(payload) < 4 {
			return nil, fmt.Errorf("serverAck payload too short for sequence: got %d bytes", len(payload))
		}
		seq := binary.BigEndian.Uint32(payload[0:4])
		result["seq"] = seq
		if len(payload) >= 8 { // If there's more data, assume it's payload size and then payload
			payloadSize = int32(binary.BigEndian.Uint32(payload[4:8]))
			if len(payload) < 8+int(payloadSize) {
				return nil, fmt.Errorf("serverAck payload too short for declared payload size: got %d bytes, expected header + %d bytes", len(payload), payloadSize)
			}
			payloadMsg = payload[8:]
		} else {
			// serverAck might not have a payload body, only sequence
			payloadSize = 0
			payloadMsg = nil
		}
	case serverErrorResponse:
		code := uint32(binary.BigEndian.Uint32(payload[:4]))
		result["code"] = code
		payloadSize = int32(binary.BigEndian.Uint32(payload[4:8]))
		payloadMsg = payload[8:]
	}

	if payloadMsg != nil {
		if compressionMethod == gzipCompression {
			reader, err := gzip.NewReader(bytes.NewReader(payloadMsg))
			if err != nil {
				return nil, fmt.Errorf("解压响应数据失败: %v", err)
			}
			defer reader.Close()

			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(reader); err != nil {
				return nil, fmt.Errorf("读取解压数据失败: %v", err)
			}
			payloadMsg = buf.Bytes()
		}

		if serializationMethod == jsonFormat {
			var jsonData map[string]interface{}
			if err := json.Unmarshal(payloadMsg, &jsonData); err != nil {
				return nil, fmt.Errorf("解析JSON响应失败: %v", err)
			}
			result["payload_msg"] = jsonData
		} else if serializationMethod != noSerialization {
			result["payload_msg"] = string(payloadMsg)
		}
	}

	result["payload_size"] = payloadSize
	return result, nil
}

// AddAudio 添加音频数据到缓冲区
func (p *Provider) AddAudio(data []byte) error {
	return p.AddAudioWithContext(context.Background(), data)
}

// AddAudioWithContext 带上下文的音频数据添加
func (p *Provider) AddAudioWithContext(ctx context.Context, data []byte) error {
	// 使用锁检查状态
	p.connMutex.Lock()
	isStreaming := p.isStreaming
	p.connMutex.Unlock()

	if !isStreaming {
		fmt.Print("开始流式识别\n")
		// 加锁保护连接初始化
		p.connMutex.Lock()
		defer p.connMutex.Unlock()

		// 双重检查，避免并发初始化
		if p.isStreaming {
			return nil
		}

		// 初始化流式识别
		p.InitAudioProcessing()
		p.result = ""
		p.err = nil

		// 确保旧连接已关闭
		if p.conn != nil {
			p.closeConnection()
		}

		// 建立WebSocket连接
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second, // 设置握手超时
		}
		headers := map[string][]string{
			"X-Api-App-Key":     {p.appID},
			"X-Api-Access-Key":  {p.accessToken},
			"X-Api-Resource-Id": {"volc.bigasr.sauc.duration"},
			"X-Api-Connect-Id":  {p.connectID},
		}

		// 重试机制
		var conn *websocket.Conn
		var resp *http.Response
		var err error
		maxRetries := 2

		for i := 0; i <= maxRetries; i++ {
			conn, resp, err = dialer.DialContext(ctx, p.wsURL, headers)
			if err == nil {
				break
			}

			if i < maxRetries {
				backoffTime := time.Duration(500*(i+1)) * time.Millisecond
				fmt.Printf("WebSocket连接失败(尝试%d/%d): %v, 将在%v后重试\n",
					i+1, maxRetries+1, err, backoffTime)
				time.Sleep(backoffTime)
			}
		}

		if err != nil {
			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode
			}
			return fmt.Errorf("WebSocket连接失败(状态码:%d): %v", statusCode, err)
		}

		p.conn = conn

		// 发送初始请求
		p.reqID = fmt.Sprintf("%d", time.Now().UnixNano())
		request := p.constructRequest()
		requestBytes, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("构造请求数据失败: %v", err)
		}

		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		if _, err := gzipWriter.Write(requestBytes); err != nil {
			return fmt.Errorf("压缩请求数据失败: %v", err)
		}
		gzipWriter.Close()

		compressedRequest := buf.Bytes()
		header := p.generateHeader(clientFullRequest, noSequence, jsonFormat)

		// 构造完整请求
		size := make([]byte, 4)
		binary.BigEndian.PutUint32(size, uint32(len(compressedRequest)))
		fullRequest := append(header, size...)
		fullRequest = append(fullRequest, compressedRequest...)

		// 发送请求
		if err := p.conn.WriteMessage(websocket.BinaryMessage, fullRequest); err != nil {
			return fmt.Errorf("发送请求失败: %v", err)
		}

		// 读取响应
		_, response, err := p.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("读取响应失败: %v", err)
		}

		initialResult, err := p.parseResponse(response)
		if err != nil {
			return fmt.Errorf("解析响应失败: %v", err)
		}

		// 检查初始响应状态
		if msg, ok := initialResult["payload_msg"].(map[string]interface{}); ok {
			// Doubao ASR v3 uses 20000000 for success code in initial response
			if code, ok := msg["code"].(float64); ok && int(code) != 20000000 {
				return fmt.Errorf("ASR初始化错误: %v", msg)
			}
		}

		p.isStreaming = true
		// 开启一个协程来处理响应，读取最后的结果，读取完成后关闭协程
		go func() {
			for {
				_, response, err := p.conn.ReadMessage()
				if err != nil {
					p.connMutex.Lock()
					p.err = fmt.Errorf("读取响应失败: %v", err)
					p.connMutex.Unlock()
					return
				}

				result, err := p.parseResponse(response)
				if err != nil {
					p.connMutex.Lock()
					p.err = fmt.Errorf("解析响应失败: %v", err)
					p.connMutex.Unlock()
					return
				}

				var respPayload responsePayload

				payloadMsgData, err := json.Marshal(result["payload_msg"])
				if err != nil {
					p.connMutex.Lock()
					p.err = fmt.Errorf("重新序列化响应payload_msg失败: %v", err)
					p.connMutex.Unlock()
					return
				}

				if err := json.Unmarshal(payloadMsgData, &respPayload); err != nil {
					p.connMutex.Lock()
					p.err = fmt.Errorf("解析最终响应payload失败: %v. Raw: %s", err, string(payloadMsgData))
					p.connMutex.Unlock()
					return
				}

				if respPayload.Code == 20000000 || respPayload.Code == 0 {
					p.connMutex.Lock()
					p.result = respPayload.Result.Text
					p.connMutex.Unlock()

					if listener := p.BaseProvider.GetListener(); listener != nil {
						if finished := listener.OnAsrResult(respPayload.Result.Text); finished {
							return
						}
					}
				}
			}
		}()
	}

	// 直接处理传入的音频数据，不使用缓冲区
	now := time.Now()
	p.BaseProvider.SetLastChunkTime(now)

	// 检查是否有实际数据需要发送
	if len(data) > 0 && p.isStreaming {
		// 直接发送音频数据
		if err := p.sendAudioData(data, false); err != nil {
			return fmt.Errorf("发送音频数据失败: %v", err)
		}
	}

	// 检查是否超时
	if p.isStreaming && now.Sub(p.GetLastChunkTime()) > idleTimeout {
		fmt.Println("超时, 发送最后的音频数据")
		if err := p.sendAudioData(data, true); err != nil {
			return fmt.Errorf("发送最后的音频数据失败: %v", err)
		}
		p.Reset()
	}

	return nil
}

func (p *Provider) closeConnection() {
	if p.conn != nil {
		_ = p.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

		// 尝试发送关闭消息
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closing connection")
		_ = p.conn.WriteMessage(websocket.CloseMessage, closeMsg)

		// 关闭连接
		_ = p.conn.Close()
		p.conn = nil

		time.Sleep(50 * time.Millisecond)
	}
}

// sendAudioData 直接发送音频数据，替代之前的sendCurrentBuffer
func (p *Provider) sendAudioData(data []byte, isLast bool) error {
	// 如果没有数据且不是最后一帧，不发送
	if len(data) == 0 && !isLast {
		return nil
	}

	var compressBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressBuffer)
	if _, err := gzipWriter.Write(data); err != nil {
		return fmt.Errorf("压缩音频数据失败: %v", err)
	}
	gzipWriter.Close()

	compressedAudio := compressBuffer.Bytes()
	flags := uint8(0)
	if isLast {
		flags = negSequence
	}

	header := p.generateHeader(clientAudioRequest, flags, noSerialization)
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(compressedAudio)))

	audioMessage := append(header, size...)
	audioMessage = append(audioMessage, compressedAudio...)

	if err := p.conn.WriteMessage(websocket.BinaryMessage, audioMessage); err != nil {
		return fmt.Errorf("发送音频数据失败: %v", err)
	}

	return nil
}

// Reset 重置ASR状态
func (p *Provider) Reset() error {
	// 使用锁保护状态变更
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.isStreaming = false
	p.closeConnection()

	p.reqID = ""
	p.result = ""
	p.err = nil

	// 重置音频处理
	p.InitAudioProcessing()

	if p.logger != nil {
		p.logger.Info("ASR状态已重置")
	}
	return nil
}

// Initialize 实现Provider接口的Initialize方法
func (p *Provider) Initialize() error {
	// 确保输出目录存在
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("初始化输出目录失败: %v", err)
	}
	return nil
}

// Cleanup 实现Provider接口的Cleanup方法
func (p *Provider) Cleanup() error {
	// 使用锁保护状态变更
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	// 确保WebSocket连接关闭
	p.closeConnection()

	if p.logger != nil {
		p.logger.Info("ASR资源已清理")
	}
	return nil
}

// Finalize 实现ASRProvider接口的Finalize方法，完成最终处理
func (p *Provider) Finalize() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	// 如果不是流式识别状态，直接返回
	if !p.isStreaming || p.conn == nil {
		return nil
	}

	// 发送一个空音频帧但标记为最后一帧，触发最终识别结果
	if err := p.sendAudioData([]byte{}, true); err != nil {
		return fmt.Errorf("发送最终帧失败: %v", err)
	}

	return nil
}

func init() {
	// 注册豆包ASR提供者
	asr.Register("doubao", func(config *asr.Config, deleteFile bool) (asr.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}
