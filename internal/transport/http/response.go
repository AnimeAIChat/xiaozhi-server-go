package httptransport

import "github.com/gin-gonic/gin"

// APIResponse 定义统一的接口返回结构体
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
	Code    int         `json:"code"`
}

// RespondSuccess 返回成功响应
func RespondSuccess(c *gin.Context, httpStatus int, data interface{}, message string) {
	if message == "" {
		message = "ok"
	}

	resp := APIResponse{
		Success: true,
		Message: message,
		Code:    httpStatus,
		Data:    data,
	}

	c.JSON(httpStatus, resp)
}

// RespondError 返回失败响应
func RespondError(c *gin.Context, httpStatus int, message string, data interface{}) {
	resp := APIResponse{
		Success: false,
		Message: message,
		Code:    httpStatus,
		Data:    data,
	}

	c.JSON(httpStatus, resp)
}