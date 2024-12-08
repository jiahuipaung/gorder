package middleware

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

func RequestLog(l *logrus.Entry) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestIn(c, l)
		defer requestOut(c, l)
		c.Next()
	}
}

func requestIn(c *gin.Context, l *logrus.Entry) {
	c.Set("request_start", time.Now())
	body := c.Request.Body
	bodyBytes, _ := io.ReadAll(body)
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	var compactedJson bytes.Buffer
	_ = json.Compact(&compactedJson, bodyBytes)
	l.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"start": time.Now().Unix(),
		"args":  compactedJson.String(),
		"from":  c.RemoteIP(),
		"uri":   c.Request.RequestURI,
	}).Infof("_request_in")
}

func requestOut(c *gin.Context, l *logrus.Entry) {
	resp, _ := c.Get("response")
	requestStart, _ := c.Get("request_start")
	requestStartTime := requestStart.(time.Time)

	l.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"proc_time_ms": time.Since(requestStartTime).Milliseconds(),
		"response":     resp,
	}).Infof("_request_out")
}
