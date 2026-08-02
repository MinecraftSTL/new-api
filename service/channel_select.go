package service

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// CacheGetRandomSatisfiedChannel recalculates channel eligibility on every
// attempt. Attempted channel IDs are request-global, including across auto
// groups, so one channel cannot receive an extra attempt through another group.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	attemptedChannelIDs := make(map[int]struct{})
	for _, channelIDText := range param.Ctx.GetStringSlice("use_channel") {
		channelID, err := strconv.Atoi(channelIDText)
		if err == nil && channelID > 0 {
			attemptedChannelIDs[channelID] = struct{}{}
		}
	}

	if param.TokenGroup != "auto" {
		channel, err := model.GetRandomSatisfiedChannel(
			param.TokenGroup,
			param.ModelName,
			attemptedChannelIDs,
			param.RequestPath,
			true,
		)
		return channel, param.TokenGroup, err
	}

	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
	if len(autoGroups) == 0 {
		return nil, param.TokenGroup, errors.New("auto groups is not enabled")
	}

	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	currentGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyAutoGroup)
	if !crossGroupRetry && currentGroup != "" {
		channel, err := model.GetRandomSatisfiedChannel(
			currentGroup,
			param.ModelName,
			attemptedChannelIDs,
			param.RequestPath,
			true,
		)
		if err != nil {
			return nil, currentGroup, err
		}
		if channel != nil {
			logger.LogDebug(param.Ctx, "Auto selected current group without cross-group retry: %s", currentGroup)
		}
		return channel, currentGroup, nil
	}

	// First scan every usable group for an untried channel. Repeating the lowest
	// layer here would prevent later groups from ever receiving an attempt.
	for _, autoGroup := range autoGroups {
		channel, err := model.GetRandomSatisfiedChannel(
			autoGroup,
			param.ModelName,
			attemptedChannelIDs,
			param.RequestPath,
			false,
		)
		if err != nil {
			return nil, autoGroup, err
		}
		if channel == nil {
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		logger.LogDebug(param.Ctx, "Auto selected group with an untried channel: %s", autoGroup)
		return channel, autoGroup, nil
	}

	// Every current candidate has been tried. Find the last currently usable
	// group and repeat within its current lowest-priority layer.
	for i := len(autoGroups) - 1; i >= 0; i-- {
		autoGroup := autoGroups[i]
		channel, err := model.GetRandomSatisfiedChannel(
			autoGroup,
			param.ModelName,
			attemptedChannelIDs,
			param.RequestPath,
			true,
		)
		if err != nil {
			return nil, autoGroup, err
		}
		if channel == nil {
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		logger.LogDebug(param.Ctx, "Auto groups exhausted; repeating the lowest priority in group: %s", autoGroup)
		return channel, autoGroup, nil
	}

	return nil, param.TokenGroup, nil
}
