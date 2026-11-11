package api

import "github.com/gin-gonic/gin"

// 前端多语言错误码：后端只返回稳定的机器可读 code，具体文案由前端按语言映射。
// 同时保留 error 字段（英文简述），便于日志与未升级客户端兼容。
const (
	codeBadRequest       = "BAD_REQUEST"
	codeUnauthorized     = "UNAUTHORIZED"
	codeInvalidID        = "INVALID_ID"
	codeInvalidTrackID   = "INVALID_TRACK_ID"
	codeTrackNotFound    = "TRACK_NOT_FOUND"
	codeFileMissing      = "FILE_MISSING"
	codeScanInProgress   = "SCAN_IN_PROGRESS"
	codeQueryFailed      = "QUERY_FAILED"
	codeDirQueryFailed   = "DIR_QUERY_FAILED"
	codeEmptyName        = "EMPTY_NAME"
	codeNameExists       = "NAME_EXISTS"
	codeNotFound         = "NOT_FOUND"
	codeInvalidPassword  = "INVALID_PASSWORD"
	codeTooManyAttempts  = "TOO_MANY_ATTEMPTS"
)

// apiErr 统一的错误响应：code 供前端映射多语言文案，error 为英文简述，msg 携带可选细节
func apiErr(c *gin.Context, status int, code, msg string) {
	body := gin.H{"code": code, "error": code}
	if msg != "" {
		body["msg"] = msg
	}
	c.JSON(status, body)
}
