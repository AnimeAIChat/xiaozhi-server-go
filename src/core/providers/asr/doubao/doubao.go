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
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/internal/transport/ws"
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
	idleTimeout = 30 * time.Second // 没有新数据就结束识别
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
	session       *ws.Session

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

	sendDataCnt int // 计数器，用于跟踪发送的音频数据包数量
	ticker      *time.Ticker // 用于定期发送心跳包保持连接
	tickerDone  chan struct{} // 用于停止ticker的信号通道

	// 异步初始化相关字段
	initDone     chan struct{} // 初始化完成信号
	initErr      error         // 初始化错误
	isReady      bool          // 是否准备就绪
	initMutex    sync.RWMutex  // 保护初始化状态

	// 预连接相关字段
	preConn        *websocket.Conn // 预连接的WebSocket连接
	preConnReady   bool            // 预连接是否准备就绪
	preConnMutex   sync.RWMutex    // 保护预连接状态
	preConnCtx     context.Context // 预连接上下文
	preConnCancel  context.CancelFunc // 预连接取消函数
}

// NewProvider 创建豆包ASR提供者实例
func NewProvider(config *asr.Config, deleteFile bool, logger *utils.Logger, session *ws.Session) (*Provider, error) {
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
		outputDir = "data/tmp/"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 创建连接ID
	connectID := fmt.Sprintf("%d", time.Now().UnixNano())
	// 从配置中读取end_window_size，如果没有设置则使用默认值400
	endWindowSize := 400 // 默认值
	if configEndWindowSize, ok := config.Data["end_window_size"]; ok {
		switch v := configEndWindowSize.(type) {
		case float64:
			endWindowSize = int(v)
		case int:
			endWindowSize = v
		case int64:
			endWindowSize = int(v)
		}
	}

	url := "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream"
	provider := &Provider{
		BaseProvider:  base,
		appID:         appID,
		accessToken:   accessToken,
		outputDir:     outputDir,
		host:          "openspeech.bytedance.com",
		wsURL:         url,
		chunkDuration: 200, // 固定使用200ms分片
		connectID:     connectID,
		logger:        logger, // 使用简单的logger
		session:       session, // session 可以为 nil

		// 默认配置
		modelName:     "bigmodel",
		endWindowSize: endWindowSize,
		enablePunc:    true,
		enableITN:     true,
		enableDDC:     false,

		// 初始化异步字段
		initDone: make(chan struct{}),
		isReady:  false,

		// 初始化预连接字段
		preConnReady: false,
	}

	// 初始化音频处理
	provider.InitAudioProcessing()

	// 注释掉预连接启动，改为按需连接
	// provider.startPreConnect()

	return provider, nil
}

// Transcribe 实现asr.Provider接口的转录方法
func (p *Provider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if p.isStreaming {
		return "", fmt.Errorf("正在进行流式识别, 请先调用Reset")
	}

	// 创建临时文件
	tempFile := filepath.Join(p.outputDir, fmt.Sprintf("temp_%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(tempFile, audioData, 0o644); err != nil {
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
func (p *Provider) generateHeader(
	messageType uint8,
	flags uint8,
	serializationMethod uint8,
) []byte {
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

type AsrResponsePayload struct {
	AudioInfo struct {
		Duration int `json:"duration"`
	} `json:"audio_info"`
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Definite  bool   `json:"definite"`
			EndTime   int    `json:"end_time"`
			StartTime int    `json:"start_time"`
			Text      string `json:"text"`
			Words     []struct {
				EndTime   int    `json:"end_time"`
				StartTime int    `json:"start_time"`
				Text      string `json:"text"`
			} `json:"words"`
		} `json:"utterances,omitempty"`
	} `json:"result"`
	Error string `json:"error,omitempty"`
}

type AsrResponse struct {
	Code            int                 `json:"code"`
	Event           int                 `json:"event"`
	IsLastPackage   bool                `json:"is_last_package"`
	PayloadSequence int32               `json:"payload_sequence"`
	PayloadSize     int                 `json:"payload_size"`
	PayloadMsg      *AsrResponsePayload `json:"payload_msg"`
}

// parseResponse 解析响应数据
func (p *Provider) parseResponse(data []byte) (map[string]interface{}, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("响应数据太短")
	}
	headerSize := data[0] & 0x0f
	messageType := data[1] >> 4
	messageTypeSpecificFlags := data[1] & 0x0f // flags
	serializationMethod := data[2] >> 4
	compressionMethod := data[2] & 0x0f
	var asr_result AsrResponse
	// 解析头部
	_ = data[0] >> 4 // protocol version

	var payload []byte
	// 跳过头部获取payload
	if len(data) > 8 && data[8] == '{' {
		payload = data[8:]
		// p.logger.Info("[DEBUG] payload偏移修正为data[8:]，首字节=%d", payload[0])
	} else {
		payload = data[headerSize*4:]
	}
	result := make(map[string]interface{})

	if messageTypeSpecificFlags&0x01 != 0 {
		asr_result.PayloadSequence = int32(binary.BigEndian.Uint32(payload[:4]))
	}
	if messageTypeSpecificFlags&0x02 != 0 {
		asr_result.IsLastPackage = true
		result["is_last_package"] = true
		//p.logger.Info("收到最后一个包, PayloadSequence=%d", asr_result.PayloadSequence)
	}
	if messageTypeSpecificFlags&0x04 != 0 {
		asr_result.Event = int(binary.BigEndian.Uint32(payload[:4]))
	}

	var payloadMsg []byte
	var payloadSize int32

	switch messageType {
	case serverFullResponse:
		// 如果 payload 直接是 JSON（如以 '{' 开头），直接解析，不做 sequence/payloadSize 处理
		if len(payload) > 0 && payload[0] == '{' {
			// p.logger.Info("[DEBUG] 进入JSON直解析分支，payload长度=%d", len(payload))
			payloadMsg = payload
			payloadSize = int32(len(payload))
		} else {
			// p.logger.Info("[DEBUG] 进入协议头解析分支，payload长度=%d", len(payload))
			// Doc: Header | Sequence | Payload size | Payload
			if len(payload) < 8 {
				return nil, fmt.Errorf("serverFullResponse payload too short for sequence and size: got %d bytes", len(payload))
			}
			seq := binary.BigEndian.Uint32(payload[0:4])
			result["seq"] = seq // Store WebSocket frame sequence
			payloadSize = int32(binary.BigEndian.Uint32(payload[4:8]))
			if len(payload) < 8+int(payloadSize) {
				return nil, fmt.Errorf("serverFullResponse payload too short for declared payload size: got %d bytes, expected header + %d bytes", len(payload), payloadSize)
			}
			payloadMsg = payload[8:]
		}
	case serverAck:
		// Doc for serverAck is not detailed for ASR, but generally it might have a sequence
		if len(payload) < 4 {
			return nil, fmt.Errorf(
				"serverAck payload too short for sequence: got %d bytes",
				len(payload),
			)
		}
		seq := binary.BigEndian.Uint32(payload[0:4])
		result["seq"] = seq
		if len(payload) >= 8 { // If there's more data, assume it's payload size and then payload
			payloadSize = int32(binary.BigEndian.Uint32(payload[4:8]))
			if len(payload) < 8+int(payloadSize) {
				return nil, fmt.Errorf(
					"serverAck payload too short for declared payload size: got %d bytes, expected header + %d bytes",
					len(payload),
					payloadSize,
				)
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
	asr_result.PayloadSize = int(payloadSize)
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
			p.logger.DebugTag("ASR", "解析响应成功，数据=%v", jsonData)
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
		err := p.StartStreaming(ctx)
		if err != nil {
			return err
		}
	}

	// 等待异步初始化完成
	p.initMutex.RLock()
	initDone := p.initDone
	p.initMutex.RUnlock()

	if initDone != nil {
		select {
		case <-initDone:
			// 初始化完成，检查是否有错误
			p.initMutex.RLock()
			initErr := p.initErr
			p.initMutex.RUnlock()
			if initErr != nil {
				return initErr
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second): // 超时保护
			return fmt.Errorf("ASR初始化超时")
		}
	}

	// 检查是否有实际数据需要发送
	if len(data) > 0 && p.isStreaming {
		// 直接发送音频数据
		if err := p.sendAudioData(data, false); err != nil {
			return err
		} else {
			p.sendDataCnt += 1
			if p.sendDataCnt%20 == 0 {
				p.logger.Debug("发送音频数据成功, 长度: %d 字节", len(data))
			}
		}
	}

	return nil
}

func (p *Provider) StartStreaming(ctx context.Context) error {
	p.logger.InfoTag("ASR", "流式识别开始")
	p.ResetStartListenTime()

	// 调用 session 的 ResetLLMContext 方法
	if p.session != nil {
		p.session.ResetLLMContext()
	}

	// 检查是否已经在初始化或准备就绪
	p.initMutex.RLock()
	if p.isReady || p.initDone == nil {
		p.initMutex.RUnlock()
		return nil
	}
	p.initMutex.RUnlock()

	// 加锁保护初始化过程
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	// 双重检查，避免并发初始化
	if p.isStreaming {
		return nil
	}

	// 初始化流式识别状态
	p.InitAudioProcessing()
	p.result = ""
	p.err = nil

	// 异步初始化WebSocket连接
	go p.asyncInitialize(ctx)

	// 立即返回，不等待初始化完成
	p.isStreaming = true
	p.logger.DebugTag("ASR", "开始异步初始化WebSocket连接")

	return nil
}

// startPreConnect 启动预连接
func (p *Provider) startPreConnect() {
	p.preConnCtx, p.preConnCancel = context.WithCancel(context.Background())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.logger.Error("预连接发生错误: %v", r)
			}
		}()

		p.logger.DebugTag("ASR", "开始预连接到Doubao ASR服务")

		// 建立WebSocket连接
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
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
			conn, resp, err = dialer.DialContext(p.preConnCtx, p.wsURL, headers)
			if err == nil {
				break
			}

			// 检查是否是401认证错误
			if resp != nil && resp.StatusCode == 401 {
				p.logger.Warn("预连接认证失败，跳过预连接")
				return
			}

			if i < maxRetries {
				backoffTime := time.Duration(500*(i+1)) * time.Millisecond
				p.logger.Debug("预连接失败(尝试%d/%d): %v, 将在%v后重试",
					i+1, maxRetries+1, err, backoffTime)

				select {
				case <-p.preConnCtx.Done():
					return // 上下文取消，退出
				case <-time.After(backoffTime):
					// 继续重试
				}
			}
		}

		if err != nil {
			p.logger.Warn("预连接失败，将在需要时再连接: %v", err)
			return
		}

		// 设置预连接
		p.preConnMutex.Lock()
		p.preConn = conn
		p.preConnReady = true
		p.preConnMutex.Unlock()

		p.logger.InfoTag("ASR", "预连接到Doubao ASR服务成功")

		// 保持预连接活跃，定期发送心跳
		ticker := time.NewTicker(30 * time.Second) // 每30秒发送一次心跳
		defer ticker.Stop()

		for {
			select {
			case <-p.preConnCtx.Done():
				// 上下文取消，关闭预连接
				p.preConnMutex.Lock()
				if p.preConn != nil {
					p.preConn.Close()
					p.preConn = nil
				}
				p.preConnReady = false
				p.preConnMutex.Unlock()
				return

			case <-ticker.C:
				// 发送心跳包保持连接
				p.preConnMutex.Lock()
				if p.preConn != nil && p.preConnReady {
					// 发送一个小的ping消息
					if err := p.preConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
						p.logger.Warn("预连接心跳失败: %v", err)
						p.preConn.Close()
						p.preConn = nil
						p.preConnReady = false
					}
				}
				p.preConnMutex.Unlock()
			}
		}
	}()
}

// sendInitialRequest 发送初始请求并等待响应
func (p *Provider) sendInitialRequest(ctx context.Context) {
	// 发送初始请求
	p.reqID = fmt.Sprintf("%d", time.Now().UnixNano())
	request := p.constructRequest()
	requestBytes, err := json.Marshal(request)
	if err != nil {
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("构造请求数据失败: %v", err)
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	if _, err := gzipWriter.Write(requestBytes); err != nil {
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("压缩请求数据失败: %v", err)
		p.initMutex.Unlock()
		close(p.initDone)
		return
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
	p.connMutex.Lock()
	if p.conn == nil {
		p.connMutex.Unlock()
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("连接已关闭")
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}
	err = p.conn.WriteMessage(websocket.BinaryMessage, fullRequest)
	p.connMutex.Unlock()

	if err != nil {
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("发送请求失败: %v", err)
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}

	// 读取响应
	p.connMutex.Lock()
	if p.conn == nil {
		p.connMutex.Unlock()
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("连接已关闭")
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}
	_, response, err := p.conn.ReadMessage()
	p.connMutex.Unlock()

	if err != nil {
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("读取响应失败: %v", err)
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}

	p.logger.DebugTag("ASR", "流式识别收到 WebSocket 数据长度=%d", len(response))

	initialResult, err := p.parseResponse(response)
	if err != nil {
		p.initMutex.Lock()
		p.initErr = fmt.Errorf("解析响应失败: %v", err)
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}

	// 检查初始响应状态
	if msg, ok := initialResult["payload_msg"].(map[string]interface{}); ok {
		// Doubao ASR v3 uses 20000000 for success code in initial response
		if code, ok := msg["code"].(float64); ok && int(code) != 20000000 {
			p.initMutex.Lock()
			p.initErr = fmt.Errorf("ASR初始化错误: %v", msg)
			p.initMutex.Unlock()
			close(p.initDone)
			return
		}
	}

	// 初始化成功
	p.initMutex.Lock()
	p.isReady = true
	p.initMutex.Unlock()

	p.logger.DebugTag("ASR", "流式识别初始化成功 connectID=%s reqID=%s", p.connectID, p.reqID)

	// 启动心跳包ticker，每200ms发送一次心跳包保持连接
	p.tickerDone = make(chan struct{})
	p.ticker = time.NewTicker(200 * time.Millisecond)
	go p.keepAlive()

	// 开启一个协程来处理响应，读取最后的结果，读取完成后关闭协程
	go func() {
		p.ReadMessage()
	}()

	// 关闭初始化完成信号
	close(p.initDone)
}

// asyncInitialize 异步初始化WebSocket连接
func (p *Provider) asyncInitialize(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("异步初始化发生错误: %v", r)
			p.initMutex.Lock()
			p.initErr = fmt.Errorf("异步初始化panic: %v", r)
			p.initMutex.Unlock()
			close(p.initDone)
		}
	}()

	// 确保旧连接已关闭
	if p.conn != nil {
		p.closeConnection()
	}

	// 首先尝试使用预连接
	p.preConnMutex.Lock()
	if p.preConnReady && p.preConn != nil {
		p.logger.InfoTag("ASR", "使用预连接进行初始化")
		p.conn = p.preConn
		p.preConn = nil
		p.preConnReady = false
		p.preConnMutex.Unlock()

		// 预连接已经建立，直接发送初始请求
		p.sendInitialRequest(ctx)
		return
	}
	p.preConnMutex.Unlock()

	// 没有预连接可用，正常建立连接
	p.logger.DebugTag("ASR", "没有预连接可用，正常建立连接")

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

		// 检查是否是401认证错误
		if resp != nil && resp.StatusCode == 401 {
			err = fmt.Errorf("ASR配置错误: API密钥或应用ID无效(401认证失败)，请检查配置文件中的access_token和appid")
			break
		}

		if i < maxRetries {
			backoffTime := time.Duration(500*(i+1)) * time.Millisecond
			p.logger.Debug("WebSocket连接失败(尝试%d/%d): %v, 将在%v后重试",
				i+1, maxRetries+1, err, backoffTime)
			time.Sleep(backoffTime)
		}
	}

	if err != nil {
		p.initMutex.Lock()
		p.initErr = err
		p.initMutex.Unlock()
		close(p.initDone)
		return
	}

	// 设置连接
	p.connMutex.Lock()
	p.conn = conn
	p.connMutex.Unlock()

	p.sendInitialRequest(ctx)
}

// keepAlive 定期发送心跳包保持连接活跃
func (p *Provider) keepAlive() {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("心跳包协程发生错误: %v", r)
		}
	}()

	for {
		select {
		case <-p.ticker.C:
			p.connMutex.Lock()
			if !p.isStreaming || p.conn == nil {
				p.connMutex.Unlock()
				return
			}
			p.connMutex.Unlock()

			// 发送一个空的音频包作为心跳包
			emptyAudio := []byte{}
			if err := p.sendAudioData(emptyAudio, false); err != nil {
				p.logger.Warn("发送心跳包失败: %v", err)
				// 如果心跳包发送失败，停止心跳
				return
			}
		case <-p.tickerDone:
			return
		}
	}
}

func (p *Provider) ReadMessage() {
	p.logger.InfoTag("ASR", "Doubao 流式识别协程启动")
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("流式识别协程发生错误: %v", r)
		}
		p.connMutex.Lock()
		p.isStreaming = false // 标记流式识别结束

		// 停止心跳包ticker
		if p.ticker != nil {
			p.ticker.Stop()
			p.ticker = nil
		}
		if p.tickerDone != nil {
			close(p.tickerDone)
			p.tickerDone = nil
		}

		if p.conn != nil {
			p.closeConnection()
		}
		p.connMutex.Unlock()
		p.logger.InfoTag("ASR", "Doubao 流式识别协程结束")
	}()

	for {
		// 检查连接状态，避免在连接关闭后继续读取
		p.connMutex.Lock()
		if !p.isStreaming || p.conn == nil {
			p.connMutex.Unlock()
			p.logger.Info("流式识别已结束或连接已关闭，退出读取循环")
			return
		}
		conn := p.conn
		p.connMutex.Unlock()

		_, response, err := conn.ReadMessage()
		if err != nil {
			p.setErrorAndStop(err)
			return
		}

		result, err := p.parseResponse(response)
		if err != nil {
			p.setErrorAndStop(fmt.Errorf("解析响应失败: %v", err))
			return
		}

		if code, hasCode := result["code"]; hasCode {
			p.logger.Info("检测到code字段: 解析结果=%v", result)
			codeValue := code.(uint32)
			if codeValue != 0 {
				// 对特定的超时错误进行特殊处理
				if codeValue == 45000081 {
					p.logger.Warn("检测到ASR会话超时错误(Code=45000081)，这可能是由于快速重启导致的，将尝试重新启动")
					// 不立即停止，而是标记需要重启
					p.connMutex.Lock()
					p.isStreaming = false
					p.closeConnection()
					p.connMutex.Unlock()
					return
				}
				p.setErrorAndStop(fmt.Errorf("ASR服务端错误: Code=%d", codeValue))
				return
			}
		}

		// 处理正常响应
		if payloadMsg, ok := result["payload_msg"].(map[string]interface{}); ok {
			// 检查是否有 result 字段（正常响应）
			if resultData, hasResult := payloadMsg["result"].(map[string]interface{}); hasResult {
				// 提取文本结果
				text := ""
				if textData, hasText := resultData["text"].(string); hasText {
					text = textData
				}

				p.logger.DebugTag("ASR", "识别成功，文本='%s'", text)

				p.connMutex.Lock() 
				p.result = text
				p.connMutex.Unlock()
				isLastPackage := false
				if isLast, hasLast := result["is_last_package"]; hasLast && isLast.(bool) {
					// 如果是最后一个包，结束流式识别
					isLastPackage = true
					p.logger.Info("检测到最后一个ASR语音包, is_last_package=%v", isLast)
				}

				if listener := p.BaseProvider.GetListener(); listener != nil {
					if text == "" && p.SilenceTime() > idleTimeout {
						p.BaseProvider.SilenceCount += 1
						text = "[SILENCE_TIMEOUT] 用户有一段时间没说话了，请礼貌提醒用户"
						p.logger.Info("检测到静音超时, SilenceTime=%v/%v", p.SilenceTime(), idleTimeout)
						p.ResetStartListenTime()
					} else if text != "" {
						p.BaseProvider.SilenceCount = 0 // 重置静音计数
					}
					if text == "" && !isLastPackage {
						continue
					}
					// 直接调用listener.OnAsrResult，不再通过事件总线
					listener.OnAsrResult(text, isLastPackage)
				}
			} else if errorData, hasError := payloadMsg["error"]; hasError {
				// 处理错误响应中的 error 字段
				p.setErrorAndStop(fmt.Errorf("ASR响应错误: %v", errorData))
				return
			}
		}

	}
}

func (p *Provider) setErrorAndStop(err error) {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.err = err
	p.isStreaming = false
	errMsg := err.Error()
	if strings.Contains(errMsg, "use of closed network connection") {
		p.logger.Debug("setErrorAndStop: %v, sendDataCnt=%d", err, p.sendDataCnt)
	} else {
		p.logger.Error("setErrorAndStop: %v, sendDataCnt=%d", err, p.sendDataCnt)
	}

	if p.conn != nil {
		p.closeConnection()
	}
}

func (p *Provider) closeConnection() {
	defer func() {
		if r := recover(); r != nil {
			// 静默处理panic，避免程序崩溃
			p.logger.Error("关闭连接时发生错误: %v", r)
		}
	}()

	if p.conn != nil {
		// 不发送关闭消息，直接关闭连接
		_ = p.conn.Close()
		p.conn = nil
	}
}

func (p *Provider) SendLastAudio(data []byte) error {
	return p.sendAudioData(data, true)
}

// sendAudioData 直接发送音频数据，替代之前的sendCurrentBuffer
func (p *Provider) sendAudioData(data []byte, isLast bool) error {
	p.logger.DebugTag(
		"ASR",
		"发送音频数据 数据长度=%d isLast=%t sendDataCnt=%d",
		len(data),
		isLast,
		p.sendDataCnt,
	)
	// 如果没有数据且不是最后一帧，不发送
	if len(data) == 0 && !isLast {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			// 捕获WebSocket写入时的panic，避免程序崩溃
			p.logger.Error("发送音频数据时发生panic: %v", r)
		}
	}()

	// 检查连接是否存在
	if p.conn == nil {
		return fmt.Errorf("WebSocket连接不存在")
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

	// 停止心跳包ticker
	if p.ticker != nil {
		p.ticker.Stop()
		p.ticker = nil
	}
	if p.tickerDone != nil {
		close(p.tickerDone)
		p.tickerDone = nil
	}

	p.closeConnection()

	p.reqID = ""
	p.result = ""
	p.err = nil

	// 重置异步初始化状态
	p.initMutex.Lock()
	p.isReady = false
	p.initErr = nil
	if p.initDone != nil {
		// 重新创建一个新的channel用于下次初始化
		p.initDone = make(chan struct{})
	}
	p.initMutex.Unlock()

	// 重置音频处理
	p.InitAudioProcessing()

	// 给服务端一点时间清理会话，避免立即重启导致的超时错误
	time.Sleep(time.Millisecond)

	p.logger.DebugTag("ASR", "状态已重置")

	return nil
}

// Initialize 实现Provider接口的Initialize方法
func (p *Provider) Initialize() error {
	// 确保输出目录存在
	if err := os.MkdirAll(p.outputDir, 0o755); err != nil {
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

	p.logger.InfoTag("ASR", "资源已清理")

	return nil
}

func (p *Provider) CloseConnection() error {
	return nil
}

// 添加占位实现以避免空指针错误
// 定义一个空的 SessionHandler 实现
type emptySessionHandler struct{}

func (h *emptySessionHandler) Handle() {}
func (h *emptySessionHandler) Close() {}
func (h *emptySessionHandler) GetSessionID() string { return "empty-session" }

func init() {
	// 注册豆包ASR提供者
	asr.Register("doubao", func(config *asr.Config, deleteFile bool, logger *utils.Logger) (asr.Provider, error) {
		return NewProvider(config, deleteFile, logger, nil)
	})
}
