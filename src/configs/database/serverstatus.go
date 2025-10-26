package database

import (
	"math"
	"strconv"
	"time"
	"xiaozhi-server-go/src/models"
)

func GetServerStatus() (*models.ServerStatus, error) {
	var status models.ServerStatus
	// 使用固定 ID（models.ServerStatusID），不存在则创建，确保只保存一条记录
	if err := GetDB().FirstOrCreate(&status, models.ServerStatus{ID: ServerStatusID}).Error; err != nil {
		return nil, err
	}
	return &status, nil
}

// 使用ServerStatusID，只保存一条记录
func UpdateServerStatus(CPUUsage float64, memoryUsage float64, onlineDeviceNum int, onlineSessionNum int) error {
	// 保留两位有效数字
	CPUUsageStr := strconv.FormatFloat(math.Round(CPUUsage*100)/100, 'f', 2, 64)
	memoryUsageStr := strconv.FormatFloat(math.Round(memoryUsage*100)/100, 'f', 2, 64)

	status := models.ServerStatus{
		ID:               ServerStatusID,
		CPUUsage:         CPUUsageStr,
		MemoryUsage:      memoryUsageStr,
		OnlineDeviceNum:  onlineDeviceNum,
		OnlineSessionNum: onlineSessionNum,
		UpdatedAt:        time.Now(),
	}

	// 根据固定 ID 更新或插入同一条记录
	return GetDB().Save(&status).Error
}
