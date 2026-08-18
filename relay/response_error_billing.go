package relay

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func settleBillingForResponseParseError(c *gin.Context, info *relaycommon.RelayInfo, usage any, apiErr *types.NewAPIError) *types.NewAPIError {
	if info == nil {
		return apiErr
	}
	usageDto, ok := billingUsageFromResponseParseError(usage, apiErr)
	if !ok {
		return apiErr
	}

	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName := info.OriginModelName
		originPriceData := info.PriceData
		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return apiErr
		}
		service.PostTextConsumeQuota(c, info, usageDto, nil)
		info.OriginModelName = originModelName
		info.PriceData = originPriceData
	} else if shouldUseAudioBillingForResponseError(info, usageDto) {
		service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		service.PostTextConsumeQuota(c, info, usageDto, nil)
	}

	// The upstream request has already produced billable usage. Retrying it
	// would charge twice, so only this parsed-usage case bypasses retry rules.
	return types.NewError(apiErr, apiErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
}

func billingUsageFromResponseParseError(usage any, apiErr *types.NewAPIError) (*dto.Usage, bool) {
	if apiErr == nil || apiErr.GetErrorCode() != types.ErrorCodeBadResponseBody {
		return nil, false
	}

	var usageDto *dto.Usage
	switch value := usage.(type) {
	case *dto.Usage:
		usageDto = value
	case dto.Usage:
		usageDto = &value
	default:
		return nil, false
	}
	if !service.ValidUsageForBilling(usageDto) {
		return nil, false
	}
	return usageDto, true
}

func shouldUseAudioBillingForResponseError(info *relaycommon.RelayInfo, usage *dto.Usage) bool {
	if info == nil || usage == nil {
		return false
	}

	containsAudioTokens := usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
	switch info.RelayMode {
	case relayconstant.RelayModeAudioSpeech,
		relayconstant.RelayModeAudioTranscription,
		relayconstant.RelayModeAudioTranslation:
		return containsAudioTokens
	case relayconstant.RelayModeResponses:
		return strings.HasPrefix(info.OriginModelName, "gpt-4o-audio")
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions:
		return containsAudioTokens &&
			(ratio_setting.ContainsAudioRatio(info.OriginModelName) ||
				ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName))
	default:
		return false
	}
}
