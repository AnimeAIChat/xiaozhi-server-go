package utils

import (
	"os"
	"time"
)

// GetProjectDir 获取项目根目录
func GetProjectDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// 辅助函数：返回两个时间间隔中较小的一个
func MinDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
