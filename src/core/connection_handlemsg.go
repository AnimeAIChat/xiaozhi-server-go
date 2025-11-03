package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"xiaozhi-server-go/src/core/chat"
	"xiaozhi-server-go/src/core/image"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/utils"

	"github.com/alibabacloud-go/tea/tea"
)

// handleMessage 处理接收到的消息
func (h *ConnectionHandler) handleMessage(messageType int, message []byte) error {
	switch messageType {
	case 1: // 文本消息
		h.clientTextQueue <- string(message)
		return nil
	case 2: // 二进制消息（音频数据）
		if h.clientAudioFormat == "pcm" {
			// 直接将PCM数据放入队列
			h.clientAudioQueue <- message
		} else if h.clientAudioFormat == "opus" {
			// 检查是否初始化了opus解码器
			if h.opusDecoder != nil {
				// 解码opus数据为PCM
				decodedData, err := h.opusDecoder.Decode(message)
				if err != nil {
					h.logger.Error(fmt.Sprintf("解码Opus音频失败: %v", err))
					// 即使解码失败，也尝试将原始数据传递给ASR处理
					h.clientAudioQueue <- message
				} else {
					// 解码成功，将PCM数据放入队列
					h.logger.Debug(fmt.Sprintf("Opus解码成功: %d bytes -> %d bytes", len(message), len(decodedData)))
					if len(decodedData) > 0 {
						h.clientAudioQueue <- decodedData
					}
				}
			} else {
				// 没有解码器，直接传递原始数据
				h.clientAudioQueue <- message
			}
		}
		return nil
	default:
		h.logger.Error(fmt.Sprintf("未知的消息类型: %d", messageType))
		return fmt.Errorf("未知的消息类型: %d", messageType)
	}
}

// processClientTextMessage 处理文本数据
func (h *ConnectionHandler) processClientTextMessage(ctx context.Context, text string) error {
	// 解析JSON消息
	var msgJSON interface{}
	if err := json.Unmarshal([]byte(text), &msgJSON); err != nil {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 检查是否为整数类型
	if _, ok := msgJSON.(float64); ok {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 解析为map类型处理具体消息
	msgMap, ok := msgJSON.(map[string]interface{})
	if !ok {
		return fmt.Errorf("消息格式错误")
	}

	// 根据消息类型分发处理
	msgType, ok := msgMap["type"].(string)
	if !ok {
		return fmt.Errorf("消息类型错误")
	}

	switch msgType {
	case "hello":
		return h.handleHelloMessage(msgMap)
	case "abort":
		return h.clientAbortChat()
	case "listen":
		return h.handleListenMessage(msgMap)
	case "iot":
		return h.handleIotMessage(msgMap)
	case "chat":
		return h.handleChatMessage(ctx, text)
	case "vision":
		return h.handleVisionMessage(msgMap)
	case "image":
		return h.handleImageMessage(ctx, msgMap)
	case "mcp":
		return h.mcpManager.HandleXiaoZhiMCPMessage(msgMap)
	default:
		h.logger.Warn("=== 未知消息类型 ===", map[string]interface{}{
			"unknown_type": msgType,
			"full_message": msgMap,
		})
		return fmt.Errorf("未知的消息类型: %s", msgType)
	}
}

func (h *ConnectionHandler) handleVisionMessage(msgMap map[string]interface{}) error {
	// 处理视觉消息
	cmd := msgMap["cmd"].(string)
	if cmd == "gen_pic" {
	} else if cmd == "gen_video" {
	} else if cmd == "read_img" {
	}
	return nil
}

// handleHelloMessage 处理欢迎消息
// 客户端会上传语音格式和采样率等信息
func (h *ConnectionHandler) handleHelloMessage(msgMap map[string]interface{}) error {
	h.LogInfo(fmt.Sprintf("[客户端] [hello 收到欢迎消息] %v", msgMap))
	// 获取客户端编码格式
	if audioParams, ok := msgMap["audio_params"].(map[string]interface{}); ok {
		if format, ok := audioParams["format"].(string); ok {
			h.clientAudioFormat = format
			if format == "pcm" {
				// 客户端使用PCM格式，服务端也使用PCM格式
				h.serverAudioFormat = "pcm"
			}
		}
		if sampleRate, ok := audioParams["sample_rate"].(float64); ok {
			h.clientAudioSampleRate = int(sampleRate)
		}
		if channels, ok := audioParams["channels"].(float64); ok {
			h.clientAudioChannels = int(channels)
		}
		if frameDuration, ok := audioParams["frame_duration"].(float64); ok {
			h.clientAudioFrameDuration = int(frameDuration)
		}
		h.LogInfo(fmt.Sprintf("[客户端] [音频参数 %s/%d/%d/%d]",
			h.clientAudioFormat, h.clientAudioSampleRate, h.clientAudioChannels, h.clientAudioFrameDuration))
	}
	h.sendHelloMessage()
	h.closeOpusDecoder()
	// 初始化opus解码器
	opusDecoder, err := utils.NewOpusDecoder(&utils.OpusDecoderConfig{
		SampleRate:  h.clientAudioSampleRate, // 客户端使用24kHz采样率
		MaxChannels: h.clientAudioChannels,   // 单声道音频
	})
	if err != nil {
		h.logger.Error(fmt.Sprintf("初始化Opus解码器失败: %v", err))
	} else {
		h.opusDecoder = opusDecoder
		h.LogInfo("[Opus] [解码器] 初始化成功")
	}

	return nil
}

// handleListenMessage 处理语音相关消息
func (h *ConnectionHandler) handleListenMessage(msgMap map[string]interface{}) error {

	// 处理state参数
	state, ok := msgMap["state"].(string)
	if !ok {
		return fmt.Errorf("listen消息缺少state参数")
	}

	// 处理mode参数
	if mode, ok := msgMap["mode"].(string); ok {
		h.clientListenMode = mode
		h.LogInfo(fmt.Sprintf("[客户端] [拾音模式 %s/%s]", h.clientListenMode, state))
		h.providers.asr.SetListener(h)
	}

	switch state {
	case "start":
		if h.client_asr_text != "" && h.clientListenMode == "manual" {
			h.clientAbortChat()
		}
		h.clientVoiceStop = false
		h.client_asr_text = ""
	case "stop":
		h.clientVoiceStop = true
		h.LogInfo("客户端停止语音识别")
	case "detect":
		text, hasText := msgMap["text"].(string)

		if hasText && text != "" {
			// 检查是否为系统内部消息，如果是则不触发LLM对话
			if text == "face_recognition" {
				h.LogInfo(fmt.Sprintf("[检测] [系统消息 %s] 仅记录日志，不触发对话", text))
				return nil
			}
			// 只有文本，使用普通LLM处理
			h.LogInfo(fmt.Sprintf("[检测] [纯文本消息 %s] 使用LLM处理", text))
			return h.handleChatMessage(context.Background(), text)
		} else {
			// 既没有图片也没有文本
			h.logger.Warn("detect消息既没有text也没有image参数")
			return fmt.Errorf("detect消息缺少text或image参数")
		}
	}
	return nil
}

// handleIotMessage 处理IOT设备消息
func (h *ConnectionHandler) handleIotMessage(msgMap map[string]interface{}) error {
	if descriptors, ok := msgMap["descriptors"].([]interface{}); ok {
		// 处理设备描述符
		// 这里需要实现具体的IOT设备描述符处理逻辑
		h.LogInfo(fmt.Sprintf("收到IOT设备描述符：%v", descriptors))
	}
	if states, ok := msgMap["states"].([]interface{}); ok {
		// 处理设备状态
		// 这里需要实现具体的IOT设备状态处理逻辑
		h.LogInfo(fmt.Sprintf("收到IOT设备状态：%v", states))
	}
	return nil
}

// handleImageMessage 处理图片消息
func (h *ConnectionHandler) handleImageMessage(ctx context.Context, msgMap map[string]interface{}) error {
	// 增加对话轮次
	h.talkRound++
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("开始新的图片对话轮次: %d", currentRound))

	// 检查是否有VLLLM Provider
	if h.providers.vlllm == nil {
		h.logger.Warn("未配置VLLLM服务，图片消息将被忽略")
		return h.conn.WriteMessage(1, []byte("系统暂不支持图片处理功能"))
	}

	// 解析文本内容
	text, ok := msgMap["text"].(string)
	if !ok {
		text = "请描述这张图片" // 默认提示
	}

	// 解析图片数据
	imageDataMap, ok := msgMap["image_data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("缺少图片数据")
	}

	imageData := image.ImageData{}
	if url, ok := imageDataMap["url"].(string); ok {
		imageData.URL = url
	}
	if data, ok := imageDataMap["data"].(string); ok {
		imageData.Data = data
	}
	if format, ok := imageDataMap["format"].(string); ok {
		imageData.Format = format
	}

	// 验证图片数据
	if imageData.URL == "" && imageData.Data == "" {
		return fmt.Errorf("图片数据为空")
	}

	h.LogInfo(fmt.Sprintf("收到图片消息 %v", map[string]interface{}{
		"text":        text,
		"has_url":     imageData.URL != "",
		"has_data":    imageData.Data != "",
		"format":      imageData.Format,
		"data_length": len(imageData.Data),
	}))

	// 立即发送STT消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.logger.Error(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送TTS开始状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送思考状态的情绪
	if err := h.sendEmotionMessage("thinking"); err != nil {
		h.logger.Error(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
		return fmt.Errorf("发送情绪消息失败: %v", err)
	}

	// 添加用户消息到对话历史（包含图片信息的描述）
	userMessage := fmt.Sprintf("%s [用户发送了一张%s格式的图片]", text, imageData.Format)
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: userMessage,
	})

	// 获取对话历史
	messages := make([]providers.Message, 0)
	for _, msg := range h.dialogueManager.GetLLMDialogue() {
		// 排除包含图片信息的最后一条消息，因为我们要用VLLLM处理
		if msg.Role == "user" && strings.Contains(msg.Content, "[用户发送了一张") {
			continue
		}
		messages = append(messages, providers.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return h.genResponseByVLLM(ctx, messages, imageData, text, currentRound)
}

// handleFaceRecognitionMessage 处理人脸识别消息
func (h *ConnectionHandler) handleFaceRecognitionMessage(msgMap map[string]interface{}) error {
	userName, ok := msgMap["user_name"].(string)
	if !ok {
		return fmt.Errorf("人脸识别消息缺少user_name字段")
	}

	confidence, _ := msgMap["confidence"].(float64)
	if confidence == 0 {
		confidence = 0.85 // 默认置信度
	}

	deviceID, _ := msgMap["device_id"].(string)

	h.LogInfo(fmt.Sprintf("[人脸识别] 识别到用户: %s (置信度: %.2f, 设备: %s)", userName, confidence, deviceID))

	// 根据K230发送的用户ID，查找数据库中对应的真实用户名
	realUserName := h.getRealUserNameByClientID(userName)
	if realUserName == "" {
		realUserName = userName // 如果找不到，使用原始用户名
		h.LogInfo(fmt.Sprintf("[人脸识别] 未找到用户 %s 的真实用户名，使用原始用户名", userName))
	} else {
		h.LogInfo(fmt.Sprintf("[人脸识别] 用户 %s 对应真实用户名: %s", userName, realUserName))
	}

	// 触发AI对话，告知机器人用户身份
	h.triggerFaceRecognitionDialogue(realUserName, confidence)

	return nil
}

// triggerFaceRecognitionDialogue 触发人脸识别后的AI对话
func (h *ConnectionHandler) triggerFaceRecognitionDialogue(userName string, confidence float64) {
	h.LogInfo(fmt.Sprintf("[人脸识别对话] 为用户 %s 触发AI对话", userName))

	// 增加对话轮次
	h.talkRound++
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("[对话] [轮次 %d] 开始人脸识别对话轮次", currentRound))

	// 构建欢迎消息
	welcomeMessage := fmt.Sprintf("你好，%s！我识别到你了，很高兴见到你！", userName)

	// 发送TTS开始状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return
	}

	// 发送思考状态的情绪
	if err := h.sendEmotionMessage("happy"); err != nil {
		h.logger.Error(fmt.Sprintf("发送情绪消息失败: %v", err))
	}

	// 添加系统消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "system",
		Content: fmt.Sprintf("用户 %s 通过人脸识别进入对话，置信度: %.2f", userName, confidence),
	})

	// 添加AI欢迎消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: welcomeMessage,
	})

	// 使用TTS生成并发送欢迎音频
	h.tts_last_text_index = 1
	h.SpeakAndPlay(welcomeMessage, 1, currentRound)
}

// handleFaceDatabaseSyncMessage 处理人脸数据库同步请求
func (h *ConnectionHandler) handleFaceDatabaseSyncMessage(msgMap map[string]interface{}) error {
	deviceID, ok := msgMap["device_id"].(string)
	if !ok {
		return fmt.Errorf("人脸数据库同步消息缺少device_id字段")
	}

	h.LogInfo(fmt.Sprintf("[人脸数据库] 设备 %s 请求同步人脸数据库", deviceID))

	// 从人脸数据库获取活跃的人脸数据
	responseBytes, err := h.faceDatabase.ToActiveJSON()
	if err != nil {
		return fmt.Errorf("获取人脸数据库数据失败: %v", err)
	}

	h.LogInfo(tea.Prettify(responseBytes))

	return h.conn.WriteMessage(1, responseBytes)
}

// handleFaceRegisterMessage 处理人脸注册消息
func (h *ConnectionHandler) handleFaceRegisterMessage(msgMap map[string]interface{}) error {
	userName, ok := msgMap["user_name"].(string)
	if !ok {
		return fmt.Errorf("人脸注册消息缺少user_name字段")
	}

	feature, ok := msgMap["feature"].(string)
	if !ok {
		return fmt.Errorf("人脸注册消息缺少feature字段")
	}

	h.LogInfo(fmt.Sprintf("[人脸注册] 用户: %s, 特征长度: %d", userName, len(feature)))

	// 直接使用客户端发送的用户名作为用户ID
	userID := userName

	// 将人脸特征保存到数据库
	err := h.faceDatabase.AddFaceWithImage(userID, userName, feature, "")
	if err != nil {
		h.LogError(fmt.Sprintf("保存人脸特征失败: %v", err))
		response := map[string]interface{}{
			"type":      "face_register_ack",
			"success":   false,
			"user_id":   userID,
			"user_name": userName,
			"message":   fmt.Sprintf("注册失败: %v", err),
		}
		responseBytes, _ := json.Marshal(response)
		return h.conn.WriteMessage(1, responseBytes)
	}

	// 返回注册成功响应
	response := map[string]interface{}{
		"type":       "face_register_ack",
		"success":    true,
		"user_id":    userID,
		"user_name":  userName,
		"message":    "注册成功",
		"created_at": time.Now().Format(time.RFC3339),
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化人脸注册响应失败: %v", err)
	}

	return h.conn.WriteMessage(1, responseBytes)
}

// handleFaceDeleteMessage 处理人脸删除消息
func (h *ConnectionHandler) handleFaceDeleteMessage(msgMap map[string]interface{}) error {
	userID, ok := msgMap["user_id"].(string)
	if !ok {
		return fmt.Errorf("人脸删除消息缺少user_id字段")
	}

	h.LogInfo(fmt.Sprintf("[人脸删除] 删除用户: %s", userID))

	// 从数据库删除指定用户的人脸数据
	err := h.faceDatabase.DeleteFace(userID)
	if err != nil {
		h.LogError(fmt.Sprintf("删除人脸数据失败: %v", err))
		response := map[string]interface{}{
			"type":    "face_delete_ack",
			"success": false,
			"user_id": userID,
			"message": fmt.Sprintf("删除失败: %v", err),
		}
		responseBytes, _ := json.Marshal(response)
		return h.conn.WriteMessage(1, responseBytes)
	}

	// 返回删除成功响应
	response := map[string]interface{}{
		"type":    "face_delete_ack",
		"success": true,
		"user_id": userID,
		"message": "删除成功",
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化人脸删除响应失败: %v", err)
	}

	return h.conn.WriteMessage(1, responseBytes)
}

// handleFaceRegisterChunkMessage 处理人脸注册分块消息
func (h *ConnectionHandler) handleFaceRegisterChunkMessage(msgMap map[string]interface{}) error {
	sessionID, ok := msgMap["session_id"].(string)
	if !ok {
		return fmt.Errorf("人脸注册分块消息缺少session_id字段")
	}

	userName, ok := msgMap["user_name"].(string)
	if !ok {
		return fmt.Errorf("人脸注册分块消息缺少user_name字段")
	}

	chunkIndexFloat, ok := msgMap["chunk_index"].(float64)
	if !ok {
		return fmt.Errorf("人脸注册分块消息缺少chunk_index字段")
	}
	chunkIndex := int(chunkIndexFloat)

	totalChunksFloat, ok := msgMap["total_chunks"].(float64)
	if !ok {
		return fmt.Errorf("人脸注册分块消息缺少total_chunks字段")
	}
	totalChunks := int(totalChunksFloat)

	featureChunk, ok := msgMap["feature_chunk"].(string)
	if !ok {
		return fmt.Errorf("人脸注册分块消息缺少feature_chunk字段")
	}

	h.LogInfo(fmt.Sprintf("[人脸注册分块] 会话: %s, 用户: %s, 块: %d/%d, 长度: %d",
		sessionID, userName, chunkIndex+1, totalChunks, len(featureChunk)))

	// 立即发送ACK，不要等到后面
	h.LogInfo(fmt.Sprintf("[分块确认] 准备发送ACK for chunk %d", chunkIndex))

	// 初始化分块数据存储
	if h.faceRegisterChunks[sessionID] == nil {
		h.faceRegisterChunks[sessionID] = make(map[int]string)
	}

	// 存储分块数据
	h.faceRegisterChunks[sessionID][chunkIndex] = featureChunk
	h.faceRegisterTotalChunks[sessionID] = totalChunks
	h.faceRegisterUserName[sessionID] = userName

	// 发送分块确认消息
	chunkAckResponse := map[string]interface{}{
		"type":        "face_register_chunk_ack",
		"session_id":  sessionID,
		"chunk_index": chunkIndex,
		"success":     true,
		"message":     fmt.Sprintf("分块 %d/%d 接收成功", chunkIndex+1, totalChunks),
	}

	chunkAckBytes, err := json.Marshal(chunkAckResponse)
	if err != nil {
		h.LogError(fmt.Sprintf("序列化分块确认响应失败: %v", err))
		return fmt.Errorf("序列化分块确认响应失败: %v", err)
	}

	h.LogInfo(fmt.Sprintf("[分块确认] ACK内容: %s", string(chunkAckBytes)))
	h.LogInfo(fmt.Sprintf("[分块确认] ACK长度: %d bytes", len(chunkAckBytes)))

	if err := h.conn.WriteMessage(1, chunkAckBytes); err != nil {
		h.LogError(fmt.Sprintf("发送分块确认消息失败: %v", err))
		return fmt.Errorf("发送分块确认消息失败: %v", err)
	}

	h.LogInfo(fmt.Sprintf("[分块确认] 会话: %s, 分块: %d/%d 确认发送成功", sessionID, chunkIndex+1, totalChunks))

	return nil
}

// handleFaceRegisterCompleteMessage 处理人脸注册完成消息
func (h *ConnectionHandler) handleFaceRegisterCompleteMessage(msgMap map[string]interface{}) error {
	sessionID, ok := msgMap["session_id"].(string)
	if !ok {
		return fmt.Errorf("人脸注册完成消息缺少session_id字段")
	}

	userName, ok := msgMap["user_name"].(string)
	if !ok {
		return fmt.Errorf("人脸注册完成消息缺少user_name字段")
	}

	// 从存储中获取总分块数
	totalChunks, exists := h.faceRegisterTotalChunks[sessionID]
	if !exists {
		h.LogError(fmt.Sprintf("人脸注册完成消息缺少分块数据: 会话 %s", sessionID))

		// 发送失败响应
		response := map[string]interface{}{
			"type":      "face_register_ack",
			"success":   false,
			"user_name": userName,
			"message":   "分块数据不存在",
		}
		responseBytes, _ := json.Marshal(response)
		return h.conn.WriteMessage(1, responseBytes)
	}

	h.LogInfo(fmt.Sprintf("[人脸注册完成] 会话: %s, 用户: %s, 总块数: %d",
		sessionID, userName, totalChunks))

	// 检查是否收到所有分块
	chunks := h.faceRegisterChunks[sessionID]
	if chunks == nil || len(chunks) != totalChunks {
		h.LogError(fmt.Sprintf("人脸注册分块不完整: 收到 %d/%d 块",
			len(chunks), totalChunks))

		// 清理分块数据
		delete(h.faceRegisterChunks, sessionID)
		delete(h.faceRegisterTotalChunks, sessionID)
		delete(h.faceRegisterUserName, sessionID)

		// 发送失败响应
		response := map[string]interface{}{
			"type":      "face_register_ack",
			"success":   false,
			"user_name": userName,
			"message":   "分块数据不完整",
		}
		responseBytes, _ := json.Marshal(response)
		return h.conn.WriteMessage(1, responseBytes)
	}

	// 组装完整的人脸特征数据
	var completeFeatureRaw []byte
	for i := 0; i < totalChunks; i++ {
		chunk, exists := chunks[i]
		if !exists {
			h.LogError(fmt.Sprintf("缺少分块 %d", i))
			// 清理分块数据
			delete(h.faceRegisterChunks, sessionID)
			delete(h.faceRegisterTotalChunks, sessionID)
			delete(h.faceRegisterUserName, sessionID)

			// 发送失败响应
			response := map[string]interface{}{
				"type":      "face_register_ack",
				"success":   false,
				"user_name": userName,
				"message":   "分块数据不完整",
			}
			responseBytes, _ := json.Marshal(response)
			return h.conn.WriteMessage(1, responseBytes)
		}

		// 解码Base64分块数据
		decodedChunk, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			h.LogError(fmt.Sprintf("分块 %d Base64解码失败: %v", i, err))
			// 清理分块数据
			delete(h.faceRegisterChunks, sessionID)
			delete(h.faceRegisterTotalChunks, sessionID)
			delete(h.faceRegisterUserName, sessionID)

			// 发送失败响应
			response := map[string]interface{}{
				"type":      "face_register_ack",
				"success":   false,
				"user_name": userName,
				"message":   fmt.Sprintf("分块 %d 数据格式错误", i),
			}
			responseBytes, _ := json.Marshal(response)
			return h.conn.WriteMessage(1, responseBytes)
		}

		// 拼接原始数据
		completeFeatureRaw = append(completeFeatureRaw, decodedChunk...)
	}

	// 将完整的原始数据编码为Base64
	completeFeature := base64.StdEncoding.EncodeToString(completeFeatureRaw)

	// 清理分块数据
	delete(h.faceRegisterChunks, sessionID)
	delete(h.faceRegisterTotalChunks, sessionID)
	delete(h.faceRegisterUserName, sessionID)

	// 处理完整的人脸特征数据
	feature := completeFeature
	h.LogInfo(fmt.Sprintf("[人脸注册] 用户: %s, 特征长度: %d", userName, len(feature)))

	// 直接使用客户端发送的用户名作为用户ID
	userID := userName

	// 将人脸特征保存到数据库
	err := h.faceDatabase.AddFaceWithImage(userID, userName, feature, "")
	if err != nil {
		h.LogError(fmt.Sprintf("保存人脸特征失败: %v", err))
		response := map[string]interface{}{
			"type":      "face_register_ack",
			"success":   false,
			"user_id":   userID,
			"user_name": userName,
			"message":   fmt.Sprintf("注册失败: %v", err),
		}
		responseBytes, _ := json.Marshal(response)
		return h.conn.WriteMessage(1, responseBytes)
	}

	// 返回注册成功响应
	response := map[string]interface{}{
		"type":       "face_register_ack",
		"success":    true,
		"user_id":    userID,
		"user_name":  userName,
		"message":    "注册成功",
		"created_at": time.Now().Format(time.RFC3339),
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化人脸注册响应失败: %v", err)
	}

	return h.conn.WriteMessage(1, responseBytes)
}
