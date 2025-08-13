package models

import (
	"time"

	"gorm.io/datatypes"
)

/*
 用于存储客户端认证信息
 ota请求下发认证信息，客户端mqtt连接后，通过clientid进行认证
*/

type AuthClient struct {
	ID        uint           `gorm:"primaryKey"`
	ClientID  string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"client_id"`
	Username  string         `gorm:"not null"                               json:"username"`
	Password  string         `gorm:"not null"                               json:"password"`
	IP        string         `                                              json:"ip"`
	DeviceID  string         `                                              json:"device_id"`
	CreatedAt time.Time      `                                              json:"created_at"`
	ExpiresAt *time.Time     `                                              json:"expires_at,omitempty"`
	Metadata  datatypes.JSON `                                              json:"metadata,omitempty"`
}
