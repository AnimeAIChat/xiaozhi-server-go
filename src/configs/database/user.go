package database

import (
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

// GetSystemSummary
func GetSystemSummary(tx *gorm.DB) (map[string]interface{}, error) {
	// 取ServerStatus
	status, err := GetServerStatus()
	if err != nil {
		return nil, err
	}
	cpuUsage := status.CPUUsage
	memoryUsage := status.MemoryUsage

	// agent总数
	var agentCount int64
	if err := tx.Model(&models.Agent{}).Count(&agentCount).Error; err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"total_agents": agentCount,
		"cpu_usage":    cpuUsage,
		"memory_usage": memoryUsage,
	}
	return summary, nil
}
