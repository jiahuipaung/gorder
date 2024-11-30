package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"time"
)

func StructuredLog(l *logrus.Entry) gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		c.Next()
		elapsed := time.Since(t)
		l.WithFields(logrus.Fields{
			"time_elapsed_ms": elapsed.Milliseconds(),
			"request_url":     c.Request.RequestURI,
			"client_ip":       c.ClientIP(),
			"full_path":       c.FullPath(),
		}).Infof("request out")
	}
}