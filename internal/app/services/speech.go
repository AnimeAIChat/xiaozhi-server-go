package services

import (
	"fmt"
	"sync/atomic"
	"time"

	domaintts "xiaozhi-server-go/internal/domain/tts"
	"xiaozhi-server-go/internal/platform/logging"
	coreproviders "xiaozhi-server-go/src/core/providers"
)// SpeechService 处理语音相关的业务逻辑
type SpeechService struct {
ttsManager  *domaintts.Manager
ttsProvider coreproviders.TTSProvider
logger      *logging.Logger

// 语音控制
serverVoiceStop int32 // 使用原子操作控制语音停止
asrPause        int32 // ASR暂停状态

// 对话轮次
talkRound       int32
roundStartTime  time.Time

// TTS状态
ttsLastTextIndex  int32
ttsLastAudioIndex int32

// 回调函数
onSpeakAndPlay func(text string, textIndex int, round int) error
onSendMessage  func(messageType int, data []byte) error
}

// SpeechConfig 语音服务配置
type SpeechConfig struct {
TTSManager  *domaintts.Manager
TTSProvider coreproviders.TTSProvider
Logger      *logging.Logger
}

// NewSpeechService 创建新的语音服务
func NewSpeechService(config *SpeechConfig) *SpeechService {
return &SpeechService{
ttsManager:  config.TTSManager,
ttsProvider: config.TTSProvider,
logger:      config.Logger,
}
}

// SetCallbacks 设置回调函数
func (s *SpeechService) SetCallbacks(
onSpeakAndPlay func(text string, textIndex int, round int) error,
onSendMessage func(messageType int, data []byte) error,
) {
s.onSpeakAndPlay = onSpeakAndPlay
s.onSendMessage = onSendMessage
}

// SpeakAndPlay 语音合成并播放
func (s *SpeechService) SpeakAndPlay(text string, textIndex int, round int) error {
if s.onSpeakAndPlay != nil {
return s.onSpeakAndPlay(text, textIndex, round)
}
return fmt.Errorf("speak and play callback not set")
}

// StopServerVoice 停止服务端语音
func (s *SpeechService) StopServerVoice() {
atomic.StoreInt32(&s.serverVoiceStop, 1)
}

// IsServerVoiceStop 检查服务端语音是否停止
func (s *SpeechService) IsServerVoiceStop() bool {
return atomic.LoadInt32(&s.serverVoiceStop) == 1
}

// GetTalkRound 获取对话轮次
func (s *SpeechService) GetTalkRound() int {
return int(atomic.LoadInt32(&s.talkRound))
}

// SetTalkRound 设置对话轮次
func (s *SpeechService) SetTalkRound(round int) {
atomic.StoreInt32(&s.talkRound, int32(round))
}

// IncrementTalkRound 增加对话轮次
func (s *SpeechService) IncrementTalkRound() int {
return int(atomic.AddInt32(&s.talkRound, 1))
}

// GetRoundStartTime 获取轮次开始时间
func (s *SpeechService) GetRoundStartTime() time.Time {
return s.roundStartTime
}

// SetRoundStartTime 设置轮次开始时间
func (s *SpeechService) SetRoundStartTime(t time.Time) {
s.roundStartTime = t
}

// PauseASR 暂停ASR
func (s *SpeechService) PauseASR() {
atomic.StoreInt32(&s.asrPause, 1)
}

// ResumeASR 恢复ASR
func (s *SpeechService) ResumeASR() {
atomic.StoreInt32(&s.asrPause, 0)
atomic.StoreInt32(&s.serverVoiceStop, 0) // 恢复时也重置语音停止状态
}

// IsASRPause 检查ASR是否暂停
func (s *SpeechService) IsASRPause() bool {
return atomic.LoadInt32(&s.asrPause) == 1
}

// GetTTSLastTextIndex 获取TTS最后文本索引
func (s *SpeechService) GetTTSLastTextIndex() int {
return int(atomic.LoadInt32(&s.ttsLastTextIndex))
}

// SetTTSLastTextIndex 设置TTS最后文本索引
func (s *SpeechService) SetTTSLastTextIndex(index int) {
atomic.StoreInt32(&s.ttsLastTextIndex, int32(index))
}

// GetTTSLastAudioIndex 获取TTS最后音频索引
func (s *SpeechService) GetTTSLastAudioIndex() int {
return int(atomic.LoadInt32(&s.ttsLastAudioIndex))
}

// SetTTSLastAudioIndex 设置TTS最后音频索引
func (s *SpeechService) SetTTSLastAudioIndex(index int) {
atomic.StoreInt32(&s.ttsLastAudioIndex, int32(index))
}
