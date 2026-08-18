package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryUsesConfiguredStatusRules(t *testing.T) {
	original := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = original })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 200, End: 200},
		{Start: 400, End: 400},
		{Start: 408, End: 408},
		{Start: 504, End: 504},
		{Start: 524, End: 524},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{200, 400, 408, 504, 524} {
		require.True(t, shouldRetry(c, types.NewOpenAIError(errors.New("upstream error"), types.ErrorCodeBadResponse, statusCode), 1))
	}
	require.False(t, shouldRetry(c, types.NewOpenAIError(errors.New("upstream error"), types.ErrorCodeBadResponse, 500), 1))
}

func TestShouldRetryTaskRelayUsesConfiguredStatusRules(t *testing.T) {
	original := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = original })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 200, End: 200},
		{Start: 400, End: 400},
		{Start: 408, End: 408},
		{Start: 504, End: 504},
		{Start: 524, End: 524},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{200, 400, 408, 504, 524} {
		taskErr := &taskdto.TaskError{Code: "upstream_error", StatusCode: statusCode}
		require.True(t, shouldRetryTaskRelay(c, 1, taskErr, 1))
	}
	require.False(t, shouldRetryTaskRelay(c, 1, &taskdto.TaskError{Code: "upstream_error", StatusCode: 500}, 1))
}

func TestShouldRetryMapsBadResponseBodyTo000(t *testing.T) {
	original := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = original })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: operation_setting.RetryStatusCodeBadResponseBody, End: operation_setting.RetryStatusCodeBadResponseBody},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	parseErr := types.NewOpenAIError(errors.New("invalid upstream response"), types.ErrorCodeBadResponseBody, 500)
	require.True(t, shouldRetry(c, parseErr, 1))

	parseErr = types.NewError(parseErr, parseErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
	require.False(t, shouldRetry(c, parseErr, 1))
}
