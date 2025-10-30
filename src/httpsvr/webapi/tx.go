package webapi

// 通用事务包装函数
import (
	"fmt"
	"net/http"
	// "xiaozhi-server-go/src/configs/database" // DISABLED: Database functionality removed

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WithTx 包装函数，执行事务操作 - DISABLED: Database functionality removed
// c: gin.Context, fn: func(tx *gorm.DB) error
func WithTx(c *gin.Context, fn func(tx *gorm.DB) error) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Transaction operations are not available"})
}

// 无c的事务包装函数 - DISABLED: Database functionality removed
func WithTxNoContext(fn func(tx *gorm.DB) error) error {
	return fmt.Errorf("database functionality removed")
}
