package configs

import (
	"gorm.io/gorm"
)

type ConfigDBInterface interface {
	GetDB() *gorm.DB
	InitServerConfig(cfgStr string) error
	UpdateServerConfig(cfgStr string) error
	LoadServerConfig() (string, error)
}
