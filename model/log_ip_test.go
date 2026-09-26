package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordLogsAlwaysPersistClientIP(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)

	const clientIP = "203.0.113.10"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.RemoteAddr = clientIP + ":4567"
	ctx.Set("username", "ip-log-owner")

	RecordErrorLog(ctx, 1, 2, "gpt-test", "test-token", "upstream failed", 3, 1, false, "default", &LogOther{})
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ChannelId: 2,
		ModelName: "gpt-test",
		TokenName: "test-token",
		Quota:     1,
		Content:   "request completed",
		TokenId:   3,
		Group:     "default",
		Other:     &LogOther{},
	})

	var logs []Log
	require.NoError(t, LOG_DB.Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, LogTypeError, logs[0].Type)
	assert.Equal(t, LogTypeConsume, logs[1].Type)
	for _, log := range logs {
		assert.Equal(t, clientIP, log.Ip)
	}
}
