// Package httpx 提供错误模型与跨切面中间件：复刻 Fastify setErrorHandler（{error} + statusCode??500）、
// @fastify/cors（origin=webOrigin、credentials=true）。
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AppError 携带 HTTP 状态码，等价 Node 的 `throw err.statusCode`。
type AppError struct {
	Status int
	Msg    string
}

func (e *AppError) Error() string { return e.Msg }

func NewError(status int, msg string) *AppError { return &AppError{Status: status, Msg: msg} }

// Fail 写 {"error": msg} 并中止处理链。
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// FailErr 将 AppError 映射为其状态码；其余一律 500。
func FailErr(c *gin.Context, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		Fail(c, ae.Status, ae.Msg)
		return
	}
	Fail(c, http.StatusInternalServerError, "服务器错误")
}

// Recovery 复刻 setErrorHandler 兜底：任何 panic → 500 {error:"服务器错误"}。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "服务器错误"})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}

// CORS 复刻 @fastify/cors { origin: webOrigin, credentials: true }。
func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			reqHeaders := c.Request.Header.Get("Access-Control-Request-Headers")
			if reqHeaders == "" {
				reqHeaders = "Content-Type"
			}
			c.Header("Access-Control-Allow-Headers", reqHeaders)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
