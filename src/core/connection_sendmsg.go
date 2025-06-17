package core

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
	"xiaozhi-server-go/src/core/utils"
)

// sendHelloMessage 发送欢迎消息
func (h *ConnectionHandler) sendHelloMessage() error {
	// 添加安全检查
	if h.conn == nil {
		return fmt.Errorf("连接对象未初始化，无法发送hello消息")
	}

	// 其他可能的 nil 检查
	if h.config == nil {
		return fmt.Errorf("配置对象未初始化")
	}

	hello := make(map[string]interface{})
	hello["type"] = "hello"
	hello["version"] = 1
	hello["transport"] = "websocket"
	hello["session_id"] = h.sessionID
	hello["audio_params"] = map[string]interface{}{
		"format":         h.serverAudioFormat,
		"sample_rate":    h.serverAudioSampleRate,
		"channels":       h.serverAudioChannels,
		"frame_duration": h.serverAudioFrameDuration,
	}
	data, err := json.Marshal(hello)
	if err != nil {
		return fmt.Errorf("序列化欢迎消息失败: %v", err)
	}

	return h.conn.WriteMessage(1, data)
}

func (h *ConnectionHandler) sendTTSMessage(state string, text string, textIndex int) error {
	// 发送TTS状态结束通知
	stateMsg := map[string]interface{}{
		"type":        "tts",
		"state":       state,
		"session_id":  h.sessionID,
		"text":        text,
		"index":       textIndex,
		"audio_codec": "opus", // 标识使用Opus编码
	}
	data, err := json.Marshal(stateMsg)
	if err != nil {
		return fmt.Errorf("序列化%s状态失败: %v", state, err)
	}
	if err := h.conn.WriteMessage(1, data); err != nil {
		return fmt.Errorf("发送%s状态失败: %v", state, err)
	}
	return nil
}

func (h *ConnectionHandler) sendSTTMessage(text string) error {
	sttMsg := map[string]interface{}{
		"type":       "stt",
		"text":       text,
		"session_id": h.sessionID,
	}
	jsonData, err := json.Marshal(sttMsg)
	if err != nil {
		return fmt.Errorf("序列化 STT 消息失败: %v", err)
	}
	if err := h.conn.WriteMessage(1, jsonData); err != nil {
		return fmt.Errorf("发送 STT 消息失败: %v", err)
	}

	return nil
}

// sendEmotionMessage 发送情绪消息
func (h *ConnectionHandler) sendEmotionMessage(emotion string) error {
	data := map[string]interface{}{
		"type":       "llm",
		"text":       utils.GetEmotionEmoji(emotion),
		"emotion":    emotion,
		"session_id": h.sessionID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化情绪消息失败: %v", err)
	}
	return h.conn.WriteMessage(1, jsonData)
}

func (h *ConnectionHandler) sendAudioMessage(filepath string, text string, textIndex int, round int) {
	bFinishSuccess := false
	defer func() {
		// 音频发送完成后，根据配置决定是否删除文件
		h.deleteAudioFileIfNeeded(filepath, "音频发送完成")

		h.LogInfo(fmt.Sprintf("TTS音频发送任务结束(%t): %s, 索引: %d/%d", bFinishSuccess, text, textIndex, h.tts_last_text_index))
		h.providers.asr.ResetStartListenTime()
		if textIndex == h.tts_last_text_index {
			h.sendTTSMessage("stop", "", textIndex)
			if h.closeAfterChat {
				h.Close()
			} else {
				h.clearSpeakStatus()
			}
		}
	}()

	if len(filepath) == 0 {
		return
	}
	// 检查轮次
	if round != h.talkRound {
		h.LogInfo(fmt.Sprintf("sendAudioMessage: 跳过过期轮次的音频: 任务轮次=%d, 当前轮次=%d, 文本=%s",
			round, h.talkRound, text))
		// 即使跳过，也要根据配置删除音频文件
		h.deleteAudioFileIfNeeded(filepath, "跳过过期轮次")
		return
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("sendAudioMessage 服务端语音停止, 不再发送音频数据：%s", text))
		// 服务端语音停止时也要根据配置删除音频文件
		h.deleteAudioFileIfNeeded(filepath, "服务端语音停止")
		return
	}

	var audioData [][]byte
	var duration float64
	var err error

	// 使用TTS提供者的方法将音频转为Opus格式
	if h.serverAudioFormat == "pcm" {
		h.LogInfo("服务端音频格式为PCM，直接发送")
		audioData, duration, err = utils.AudioToPCMData(filepath)
		if err != nil {
			h.LogError(fmt.Sprintf("音频转PCM失败: %v", err))
			return
		}
	} else if h.serverAudioFormat == "opus" {
		audioData, duration, err = utils.AudioToOpusData(filepath)
		if err != nil {
			h.LogError(fmt.Sprintf("音频转Opus失败: %v", err))
			return
		}
	}

	// 发送TTS状态开始通知
	if err := h.sendTTSMessage("sentence_start", text, textIndex); err != nil {
		h.LogError(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return
	}

	if textIndex == 1 {
		now := time.Now()
		spentTime := now.Sub(h.roundStartTime)
		h.logger.Debug("回复首句耗时 %s 第一句话【%s】, round: %d", spentTime, text, round)
	}
	h.logger.Debug("TTS发送(%s): \"%s\" (索引:%d/%d，时长:%f，帧数:%d)", h.serverAudioFormat, text, textIndex, h.tts_last_text_index, duration, len(audioData))

	// 分时发送音频数据
	if err := h.sendAudioFrames(audioData, text, round); err != nil {
		h.LogError(fmt.Sprintf("分时发送音频数据失败: %v", err))
		return
	}

	// 发送TTS状态结束通知
	if err := h.sendTTSMessage("sentence_end", text, textIndex); err != nil {
		h.LogError(fmt.Sprintf("发送TTS结束状态失败: %v", err))
		return
	}

	bFinishSuccess = true
}

// sendAudioFrames 分时发送音频帧，避免撑爆客户端缓冲区
func (h *ConnectionHandler) sendAudioFrames(audioData [][]byte, text string, round int) error {
	if len(audioData) == 0 {
		return nil
	}

	startTime := time.Now()
	playPosition := 0 // 播放位置（毫秒）

	// 预缓冲：发送前几帧，提升播放流畅度
	preBufferFrames := 3
	if len(audioData) < preBufferFrames {
		preBufferFrames = len(audioData)
	}
	preBufferTime := time.Duration(h.serverAudioFrameDuration*preBufferFrames) * time.Millisecond // 预缓冲时间（毫秒）

	// 发送预缓冲帧
	for i := 0; i < preBufferFrames; i++ {
		// 检查是否被打断
		if atomic.LoadInt32(&h.serverVoiceStop) == 1 || round != h.talkRound {
			h.LogInfo(fmt.Sprintf("音频发送被中断(预缓冲阶段): 帧=%d/%d, 文本=%s", i+1, preBufferFrames, text))
			return nil
		}

		if err := h.conn.WriteMessage(2, audioData[i]); err != nil {
			return fmt.Errorf("发送预缓冲音频帧失败: %v", err)
		}
		playPosition += h.serverAudioFrameDuration
	}

	// 发送剩余音频帧
	remainingFrames := audioData[preBufferFrames:]
	for i, chunk := range remainingFrames {
		// 检查是否被打断或轮次变化
		if atomic.LoadInt32(&h.serverVoiceStop) == 1 || round != h.talkRound {
			h.LogInfo(fmt.Sprintf("音频发送被中断: 帧=%d/%d, 文本=%s", i+preBufferFrames+1, len(audioData), text))
			return nil
		}

		// 检查连接是否关闭
		select {
		case <-h.stopChan:
			return nil
		default:
		}

		// 计算预期发送时间
		expectedTime := startTime.Add(time.Duration(playPosition)*time.Millisecond - preBufferTime)
		currentTime := time.Now()
		delay := expectedTime.Sub(currentTime)

		// 流控延迟处理
		if delay > 0 {
			// 使用简单的可中断睡眠
			ticker := time.NewTicker(10 * time.Millisecond) // 固定10ms检查间隔
			defer ticker.Stop()

			endTime := time.Now().Add(delay)
			for time.Now().Before(endTime) {
				select {
				case <-ticker.C:
					// 检查中断条件
					if atomic.LoadInt32(&h.serverVoiceStop) == 1 || round != h.talkRound {
						h.LogInfo(fmt.Sprintf("音频发送在延迟中被中断: 帧=%d/%d, 文本=%s", i+preBufferFrames+1, len(audioData), text))
						return nil
					}
				case <-h.stopChan:
					return nil
				}
			}
		}

		// 发送音频帧
		if err := h.conn.WriteMessage(2, chunk); err != nil {
			return fmt.Errorf("发送音频帧失败: %v", err)
		}

		playPosition += h.serverAudioFrameDuration
	}
	time.Sleep(preBufferTime) // 确保预缓冲时间已过
	spentTime := time.Since(startTime).Milliseconds()
	h.LogInfo(fmt.Sprintf("音频帧发送完成: 总帧数=%d, 总时长=%dms, 总耗时:%dms 文本=%s", len(audioData), playPosition, spentTime, text))
	return nil
}
