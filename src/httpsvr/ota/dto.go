package ota

import (
	"encoding/json"
	"fmt"
)

// 兼容 string/number 的类型
type StringOrNumber string

func (s *StringOrNumber) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		// 字符串
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = StringOrNumber(str)
		return nil
	}
	// 数字
	var num interface{}
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	switch v := num.(type) {
	case float64:
		*s = StringOrNumber(fmt.Sprintf("%v", v))
	case int:
		*s = StringOrNumber(fmt.Sprintf("%d", v))
	default:
		*s = ""
	}
	return nil
}

type OTARequestBody struct {
	Application struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		CompileTime string `json:"compile_time"`
		ElfSHA256   string `json:"elf_sha256"`
		IDFVersion  string `json:"idf_version"`
	} `json:"application"`
	Board struct {
		Channel int    `json:"channel"`
		IP      string `json:"ip"`
		MAC     string `json:"mac"`
		Name    string `json:"name"`
		RSSI    int    `json:"rssi"`
		SSID    string `json:"ssid"`
		Type    string `json:"type"`
	} `json:"board"`
	ChipInfo struct {
		Cores    int `json:"cores"`
		Features int `json:"features"`
		Model    int `json:"model"`
		Revision int `json:"revision"`
	} `json:"chip_info"`
	ChipModelName       string         `json:"chip_model_name"`
	FlashSize           float64        `json:"flash_size"`
	Language            string         `json:"language"`
	MacAddress          string         `json:"mac_address"`
	MinimumFreeHeapSize StringOrNumber `json:"minimum_free_heap_size"`
	OTA                 struct {
		Label string `json:"label"`
	} `json:"ota"`
	PartitionTable []struct {
		Address float64 `json:"address"`
		Label   string  `json:"label"`
		Size    float64 `json:"size"`
		Subtype int     `json:"subtype"`
		Type    int     `json:"type"`
	} `json:"partition_table"`
	UUID    string `json:"uuid"`
	Version int    `json:"version"`
}

// 命名结构体用于响应
type ServerTimeInfo struct {
	Timestamp      int64 `json:"timestamp"       example:"1720065289451"`
	TimezoneOffset int   `json:"timezone_offset" example:"480"`
}

type FirmwareInfo struct {
	Version string `json:"version" example:"1.2.4"`
	URL     string `json:"url"     example:"/ota_bin/1.2.4.bin"`
}

type WebSocketInfo struct {
	URL string `json:"url" example:"wss://your-server/ws"`
}

type MQTTInfo struct {
	Endpoint       string `json:"endpoint"        example:"mqtt://broker:1883"`
	ClientID       string `json:"client_id"       example:"CGID_test@@@mac@@@client"`
	Username       string `json:"username"        example:"base64string"`
	Password       string `json:"password"        example:"randomPwd"`
	PublishTopic   string `json:"publish_topic"   example:"device-server"`
	SubscribeTopic string `json:"subscribe_topic" example:"null"`
}

type Activation struct {
	Code    string `json:"code"                example:"543091"`
	Message string `json:"message"             example:"Anime AI Chat
543091"`
	Challenge string `json:"challenge,omitempty"` // 用于设备认证挑战
}
type OTAResponse struct {
	ServerTime ServerTimeInfo `json:"server_time"`
	Firmware   FirmwareInfo   `json:"firmware"`
	WebSocket  WebSocketInfo  `json:"websocket"`
	MQTT       *MQTTInfo      `json:"mqtt,omitempty"`
	Activation *Activation    `json:"activation,omitempty"`
}
