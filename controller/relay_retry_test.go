package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayRetryUsesConfiguredStatusRules(t *testing.T) {
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
		require.True(t, service.ShouldRetryRelayError(c, types.NewOpenAIError(errors.New("upstream error"), types.ErrorCodeBadResponse, statusCode), 1), "status %d follows the configured retry rules", statusCode)
	}
	require.False(t, service.ShouldRetryRelayError(c, types.NewOpenAIError(errors.New("upstream error"), types.ErrorCodeBadResponse, 500), 1))
}

func TestTaskRetryUsesConfiguredStatusRules(t *testing.T) {
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
		require.Equal(t, "retry", decideTaskRetry(c, taskErr, 1).Action, "status %d follows the configured retry rules", statusCode)
	}
	require.Equal(t, "stop", decideTaskRetry(c, &taskdto.TaskError{Code: "upstream_error", StatusCode: 500}, 1).Action)
}

func TestRelayRetryMapsBadResponseBodyTo000(t *testing.T) {
	original := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = original })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: operation_setting.RetryStatusCodeBadResponseBody, End: operation_setting.RetryStatusCodeBadResponseBody},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	parseErr := types.NewOpenAIError(errors.New("invalid upstream response"), types.ErrorCodeBadResponseBody, 500)
	require.True(t, service.ShouldRetryRelayError(c, parseErr, 1))

	parseErr = types.NewError(parseErr, parseErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
	require.False(t, service.ShouldRetryRelayError(c, parseErr, 1))
}

func TestAppendUsedChannelPreservesRepeatedAttempts(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.AppendUsedChannel(c, 101)
	service.AppendUsedChannel(c, 101)
	service.AppendUsedChannel(c, 202)

	require.Equal(t, []string{"101", "101", "202"}, c.GetStringSlice("use_channel"))
}
