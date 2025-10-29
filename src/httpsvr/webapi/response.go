package webapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse 定义统一的接口返回结构体
// success: 请求是否成功
// data: 业务数据内容，失败时可为空或包含错误详情
// message: 对请求结果的说明信息
// code: 与 HTTP 状态码保持一致，便于客户端判断
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
	Code    int         `json:"code"`
}

// respond 统一输出响应结构
func respond(c *gin.Context, httpStatus int, success bool, message string, data interface{}) {
	if message == "" {
		if success {
			message = "ok"
		} else {
			message = http.StatusText(httpStatus)
		}
	}

	resp := APIResponse{
		Success: success,
		Message: message,
		Code:    httpStatus,
	}

	if data == nil {
		resp.Data = gin.H{}
	} else {
		resp.Data = data
	}

	c.JSON(httpStatus, resp)
}

// respondSuccess 返回成功响应
func RespondSuccess(c *gin.Context, httpStatus int, data interface{}, message string) {
	respond(c, httpStatus, true, message, data)
}

// respondError 返回失败响应，可携带错误详情
func RespondError(c *gin.Context, httpStatus int, message string, data interface{}) {
	respond(c, httpStatus, false, message, data)
}

func respondSuccess(c *gin.Context, httpStatus int, data interface{}, message string) {
	RespondSuccess(c, httpStatus, data, message)
}

func respondError(c *gin.Context, httpStatus int, message string, data interface{}) {
	RespondError(c, httpStatus, message, data)
}
