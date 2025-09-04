package webapi

// 通用事务包装函数
import (
	"net/http"
	"xiaozhi-server-go/src/configs/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WithTx 包装函数，执行事务操作
// c: gin.Context, fn: func(tx *gorm.DB) error
func WithTx(c *gin.Context, fn func(tx *gorm.DB) error) {
	tx := database.GetTxDB()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "事务异常中断"})
		}
	}()
	if err := fn(tx); err != nil {
		tx.Rollback()
		return
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败: " + err.Error()})
		return
	}
}

// 无c的事务包装函数
func WithTxNoContext(fn func(tx *gorm.DB) error) error {
	tx := database.GetTxDB()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}
	return nil
}
