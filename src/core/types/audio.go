package types

type AudioChunk struct {
	Text      string // 关联的文本内容
	Index     int    // 文本的索引，用于标文本在本轮的顺序
	Round     int    // 轮次，用于标记对话的轮次
	Data      []byte // 音频数据
	Timestamp int64  // 时间戳，用于标记数据块的顺序
	EOF       bool   // 是否为最后一个数据块
	Encoding  string // 音频编码格式，例如 "mp3", "wav"
}
