package relay

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingUsageFromResponseParseError(t *testing.T) {
	badBodyError := types.NewError(errors.New("response conversion failed"), types.ErrorCodeBadResponseBody)

	tests := []struct {
		name      string
		usage     any
		apiErr    *types.NewAPIError
		wantValid bool
	}{
		{
			name:      "parsed token usage",
			usage:     &dto.Usage{PromptTokens: 12},
			apiErr:    badBodyError,
			wantValid: true,
		},
		{
			name: "canonical billing usage",
			usage: &dto.Usage{
				BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{CompletionTokens: 7}),
			},
			apiErr:    badBodyError,
			wantValid: true,
		},
		{
			name:      "empty usage",
			usage:     &dto.Usage{},
			apiErr:    badBodyError,
			wantValid: false,
		},
		{
			name:      "nil usage",
			usage:     nil,
			apiErr:    badBodyError,
			wantValid: false,
		},
		{
			name:      "other error code",
			usage:     &dto.Usage{PromptTokens: 12},
			apiErr:    types.NewError(errors.New("upstream error"), types.ErrorCodeBadResponse),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, ok := billingUsageFromResponseParseError(tt.usage, tt.apiErr)
			assert.Equal(t, tt.wantValid, ok)
			if tt.wantValid {
				require.NotNil(t, actual)
			} else {
				assert.Nil(t, actual)
			}
		})
	}
}

func TestPrepareRequestBillingOutputLimitsWithoutCounting(t *testing.T) {
	previous := config.GlobalConfig.ExportAllConfigs()
	oldCount, oldSensitive, oldOutput := constant.CountToken, setting.CheckSensitiveEnabled, common.PreConsumedQuota
	t.Cleanup(func() {
		constant.CountToken, setting.CheckSensitiveEnabled, common.PreConsumedQuota = oldCount, oldSensitive, oldOutput
		require.NoError(t, config.GlobalConfig.LoadFromDB(previous))
	})
	constant.CountToken, setting.CheckSensitiveEnabled, common.PreConsumedQuota = false, false, 500
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"output-limit-test":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"output-limit-test":"tier(\"output\", c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	for _, count := range []bool{false, true} {
		constant.CountToken = count
		for _, tc := range []struct {
			name    string
			request dto.Request
			want    int
		}{
			{"chat", &dto.GeneralOpenAIRequest{MaxTokens: lo.ToPtr(uint(80)), MaxCompletionTokens: lo.ToPtr(uint(100))}, 100},
			{"responses", &dto.OpenAIResponsesRequest{MaxOutputTokens: lo.ToPtr(uint(100))}, 100},
			{"claude", &dto.ClaudeRequest{MaxTokens: lo.ToPtr(uint(100))}, 100},
			{"gemini", &dto.GeminiChatRequest{GenerationConfig: dto.GeminiChatGenerationConfig{MaxOutputTokens: lo.ToPtr(uint(100))}}, 100},
			{"fallback", &dto.GeneralOpenAIRequest{}, 500},
		} {
			t.Run(fmt.Sprintf("%s/count=%t", tc.name, count), func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				info := &relaycommon.RelayInfo{Request: tc.request, OriginModelName: "output-limit-test", UserGroup: "default", UsingGroup: "default", BillingRequestInput: &billingexpr.RequestInput{}}
				// No user cache is initialized: funding rejects after the estimate is captured.
				require.NotNil(t, PrepareRequestBilling(ctx, info))
				require.NotNil(t, info.TieredBillingSnapshot)
				assert.Equal(t, tc.want, info.TieredBillingSnapshot.EstimatedCompletionTokens)
				assert.Equal(t, tc.want*15/2, info.PriceData.QuotaToPreConsume)
			})
		}
	}
}
