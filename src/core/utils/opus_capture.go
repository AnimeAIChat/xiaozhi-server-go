package utils

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	oggPageHeaderSize = 27
	oggPageBOS        = 0x02
	oggPageEOS        = 0x04
)

// SaveOpusPackets writes the exact WebSocket Opus packets into a standards-compliant
// Ogg Opus file. The result can be decoded independently with FFmpeg/libopus.
func SaveOpusPackets(outputDir, deviceID, sessionID string, sampleRate, channels, frameDuration int, packets [][]byte) (string, error) {
	if len(packets) == 0 {
		return "", fmt.Errorf("没有可保存的 Opus 包")
	}
	if sampleRate <= 0 || channels < 1 || channels > 2 || frameDuration <= 0 {
		return "", fmt.Errorf("无效的 Opus 参数：%dHz/%d 声道/%dms", sampleRate, channels, frameDuration)
	}
	if outputDir == "" {
		return "", fmt.Errorf("Opus 抓包目录不能为空")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("创建 Opus 抓包目录失败: %w", err)
	}

	name := fmt.Sprintf("opus-%s-%s-%s.ogg",
		time.Now().Format("20060102-150405.000000000"),
		sanitizeOpusCaptureName(deviceID),
		sanitizeOpusCaptureName(sessionID),
	)
	path := filepath.Join(outputDir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("创建 Opus 抓包文件失败: %w", err)
	}
	defer file.Close()

	serial := uint32(time.Now().UnixNano())
	sequence := uint32(0)
	if err := writeOggPage(file, oggPageBOS, 0, serial, sequence, opusIdentificationHeader(sampleRate, channels)); err != nil {
		return "", fmt.Errorf("写入 Opus 标识页失败: %w", err)
	}
	sequence++
	if err := writeOggPage(file, 0, 0, serial, sequence, opusCommentHeader()); err != nil {
		return "", fmt.Errorf("写入 Opus 注释页失败: %w", err)
	}
	sequence++

	// Ogg Opus uses 48 kHz units for granule positions, independently of the
	// Opus decoder output rate advertised in OpusHead.
	granuleStep := uint64(frameDuration) * 48
	granulePosition := uint64(0)
	for i, packet := range packets {
		if len(packet) == 0 {
			return "", fmt.Errorf("第 %d 个 Opus 包为空", i+1)
		}
		granulePosition += granuleStep
		headerType := byte(0)
		if i == len(packets)-1 {
			headerType = oggPageEOS
		}
		if err := writeOggPage(file, headerType, granulePosition, serial, sequence, packet); err != nil {
			return "", fmt.Errorf("写入第 %d 个 Opus 包失败: %w", i+1, err)
		}
		sequence++
	}

	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("同步 Opus 抓包文件失败: %w", err)
	}
	return path, nil
}

func opusIdentificationHeader(sampleRate, channels int) []byte {
	header := make([]byte, 19)
	copy(header, "OpusHead")
	header[8] = 1 // version
	header[9] = byte(channels)
	// pre-skip and output gain are zero: packets are captured after encoding.
	binary.LittleEndian.PutUint32(header[12:16], uint32(sampleRate))
	header[18] = 0 // mono/stereo channel mapping family
	return header
}

func opusCommentHeader() []byte {
	vendor := []byte("xiaozhi-server-go opus packet capture")
	header := make([]byte, 8+4+len(vendor)+4)
	copy(header, "OpusTags")
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(vendor)))
	copy(header[12:12+len(vendor)], vendor)
	// The trailing uint32 is the zero user-comment count.
	return header
}

func writeOggPage(w io.Writer, headerType byte, granulePosition uint64, serial, sequence uint32, packet []byte) error {
	lacing, err := oggLacingValues(len(packet))
	if err != nil {
		return err
	}
	page := make([]byte, oggPageHeaderSize+len(lacing)+len(packet))
	copy(page, "OggS")
	page[4] = 0 // stream structure version
	page[5] = headerType
	binary.LittleEndian.PutUint64(page[6:14], granulePosition)
	binary.LittleEndian.PutUint32(page[14:18], serial)
	binary.LittleEndian.PutUint32(page[18:22], sequence)
	page[26] = byte(len(lacing))
	copy(page[oggPageHeaderSize:], lacing)
	copy(page[oggPageHeaderSize+len(lacing):], packet)
	binary.LittleEndian.PutUint32(page[22:26], oggCRC32(page))
	_, err = w.Write(page)
	return err
}

func oggLacingValues(packetLength int) ([]byte, error) {
	if packetLength < 0 {
		return nil, fmt.Errorf("Opus 包长度不能为负数")
	}
	segmentCount := packetLength/255 + 1
	if segmentCount > 255 {
		return nil, fmt.Errorf("Opus 包过大，无法写入单个 Ogg 页：%d 字节", packetLength)
	}
	lacing := make([]byte, segmentCount)
	for i := 0; i < segmentCount-1; i++ {
		lacing[i] = 255
	}
	lacing[segmentCount-1] = byte(packetLength % 255)
	return lacing, nil
}

// oggCRC32 is the unreflected CRC used by Ogg pages (polynomial 0x04C11DB7).
func oggCRC32(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc ^= uint32(b) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func sanitizeOpusCaptureName(value string) string {
	if value == "" {
		return "unknown"
	}
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, value)
	if len(value) > 48 {
		value = value[:48]
	}
	return value
}
