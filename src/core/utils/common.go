package utils

import (
    "encoding/json"
    "fmt"
    "net"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

// GetLocalIP 获取本地IP地址
func GetLocalIP() string {
    addrs, err := net.InterfaceAddrs()
    if err != nil {
        return "127.0.0.1"
    }

    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                return ipnet.IP.String()
            }
        }
    }

    return "127.0.0.1"
}

// GetProjectDir 获取项目根目录
func GetProjectDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return "."
    }
    return dir
}

// EnsureDir 确保目录存在
func EnsureDir(path string) error {
    if path == "" {
        return nil
    }
    return os.MkdirAll(path, 0755)
}

// ExtractJSONFromString 从字符串中提取JSON
func ExtractJSONFromString(s string) string {
    start := strings.Index(s, "{")
    end := strings.LastIndex(s, "}")
    if start >= 0 && end > start {
        return s[start : end+1]
    }
    return ""
}

// GetStringWithoutPunctuation 移除标点符号和表情符号
func GetStringWithoutPunctuation(s string) string {
    // 移除表情符号
    emojiRegex := regexp.MustCompile(`[\x{1F600}-\x{1F64F}]|[\x{1F300}-\x{1F5FF}]|[\x{1F680}-\x{1F6FF}]|[\x{2600}-\x{26FF}]|[\x{2700}-\x{27BF}]`)
    s = emojiRegex.ReplaceAllString(s, "")

    // 移除标点符号
    punctRegex := regexp.MustCompile(`[^\p{L}\p{N}\s]`)
    s = punctRegex.ReplaceAllString(s, "")

    // 压缩空白字符
    spaceRegex := regexp.MustCompile(`\s+`)
    s = spaceRegex.ReplaceAllString(s, " ")

    return strings.TrimSpace(s)
}

// IsJSONString 判断字符串是否是JSON格式
func IsJSONString(s string) bool {
    var js map[string]interface{}
    return json.Unmarshal([]byte(s), &js) == nil
}

// ParseJSON 解析JSON字符串
func ParseJSON(s string) (map[string]interface{}, error) {
    var result map[string]interface{}
    if err := json.Unmarshal([]byte(s), &result); err != nil {
        return nil, fmt.Errorf("解析JSON失败: %v", err)
    }
    return result, nil
}

// FormartJSONString 格式化JSON字符串
func FormatJSONString(s string) (string, error) {
    var temp interface{}
    if err := json.Unmarshal([]byte(s), &temp); err != nil {
        return "", err
    }
    
    pretty, err := json.MarshalIndent(temp, "", "  ")
    if err != nil {
        return "", err
    }
    
    return string(pretty), nil
}

// GetFileExt 获取文件扩展名
func GetFileExt(filename string) string {
    return strings.ToLower(filepath.Ext(filename))
}

// FileExists 检查文件是否存在
func FileExists(filename string) bool {
    info, err := os.Stat(filename)
    if os.IsNotExist(err) {
        return false
    }
    return !info.IsDir()
}

// DirExists 检查目录是否存在
func DirExists(path string) bool {
    info, err := os.Stat(path)
    if os.IsNotExist(err) {
        return false
    }
    return info.IsDir()
}

// SplitTextByPunctuation 按标点符号分割文本
func SplitTextByPunctuation(text string) []string {
    // 定义标点符号
    punctuations := []string{"。", "？", "！", "；", "：", ".", "?", "!", ";", ":"}
    
    var result []string
    current := text
    
    for len(current) > 0 {
        // 查找最近的标点符号
        minIndex := len(current)
        for _, p := range punctuations {
            if idx := strings.Index(current, p); idx >= 0 && idx < minIndex {
                minIndex = idx + len(p)
            }
        }
        
        if minIndex == len(current) {
            // 没有找到标点符号，添加剩余文本
            if len(strings.TrimSpace(current)) > 0 {
                result = append(result, current)
            }
            break
        }
        
        // 添加包含标点符号的片段
        if len(strings.TrimSpace(current[:minIndex])) > 0 {
            result = append(result, current[:minIndex])
        }
        current = current[minIndex:]
    }
    
    return result
}

// RetryWithTimes 重试执行函数
func RetryWithTimes(times int, f func() error) error {
    var lastErr error
    for i := 0; i < times; i++ {
        if err := f(); err != nil {
            lastErr = err
            continue
        }
        return nil
    }
    return fmt.Errorf("重试%d次后仍然失败: %v", times, lastErr)
}

// GenerateTempFilename 生成临时文件名
func GenerateTempFilename(prefix, suffix string) string {
    return fmt.Sprintf("%s_%d%s", prefix, os.Getpid(), suffix)
}
