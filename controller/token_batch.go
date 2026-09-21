package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type batchUpdateTokensRequest struct {
	IDs                []int   `json:"ids"`
	Status             *int    `json:"status,omitempty"`
	RemainQuota        *int    `json:"remain_quota,omitempty"`
	UnlimitedQuota     *bool   `json:"unlimited_quota,omitempty"`
	Group              *string `json:"group,omitempty"`
	ExpiredTime        *int64  `json:"expired_time,omitempty"`
	ModelLimitsEnabled *bool   `json:"model_limits_enabled,omitempty"`
	ModelLimits        *string `json:"model_limits,omitempty"`
}

var (
	errBatchTokenStatusInvalid = errors.New("invalid token status")
	errBatchTokenExpired       = errors.New("token expired")
	errBatchTokenExhausted     = errors.New("token exhausted")
	errBatchTokenQuotaInvalid  = errors.New("invalid token quota")
	errBatchTokenExpiryInvalid = errors.New("invalid token expiry")
)

func batchUpdateTokenFailureMessage(err error) string {
	switch {
	case errors.Is(err, errBatchTokenStatusInvalid):
		return "invalid_status"
	case errors.Is(err, errBatchTokenExpired):
		return "expired"
	case errors.Is(err, errBatchTokenExhausted):
		return "exhausted"
	case errors.Is(err, errBatchTokenQuotaInvalid):
		return "invalid_quota"
	case errors.Is(err, errBatchTokenExpiryInvalid):
		return "invalid_expiry"
	default:
		return "failed"
	}
}

func applyBatchTokenChange(token *model.Token, req batchUpdateTokensRequest) error {
	if req.Status != nil {
		if *req.Status != common.TokenStatusEnabled && *req.Status != common.TokenStatusDisabled {
			return errBatchTokenStatusInvalid
		}
		token.Status = *req.Status
	}
	if req.RemainQuota != nil {
		token.RemainQuota = *req.RemainQuota
	}
	if req.UnlimitedQuota != nil {
		token.UnlimitedQuota = *req.UnlimitedQuota
	}
	if req.Group != nil {
		token.Group = *req.Group
		if token.Group != "auto" {
			token.CrossGroupRetry = false
			_ = token.SetAutoGroups(nil)
		}
	}
	if req.ExpiredTime != nil {
		token.ExpiredTime = *req.ExpiredTime
	}
	if req.ModelLimitsEnabled != nil {
		token.ModelLimitsEnabled = *req.ModelLimitsEnabled
	}
	if req.ModelLimits != nil {
		token.ModelLimits = *req.ModelLimits
	}
	if token.ModelLimits == "" {
		token.ModelLimitsEnabled = false
	}

	if token.RemainQuota < 0 && !token.UnlimitedQuota {
		return errBatchTokenQuotaInvalid
	}
	if !token.UnlimitedQuota && token.RemainQuota > maxTokenQuota() {
		return errBatchTokenQuotaInvalid
	}
	if token.ExpiredTime < -1 {
		return errBatchTokenExpiryInvalid
	}
	if token.Status == common.TokenStatusEnabled {
		if token.ExpiredTime != -1 && token.ExpiredTime <= common.GetTimestamp() {
			return errBatchTokenExpired
		}
		if token.RemainQuota <= 0 && !token.UnlimitedQuota {
			return errBatchTokenExhausted
		}
	}
	return nil
}

// UpdateTokenBatch applies an allowlisted patch to multiple tokens owned by the caller.
func UpdateTokenBatch(c *gin.Context) {
	var req batchUpdateTokensRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	ids, ok := normalizeBatchIDs(req.IDs)
	if !ok || (req.Status == nil &&
		req.RemainQuota == nil &&
		req.UnlimitedQuota == nil &&
		req.Group == nil &&
		req.ExpiredTime == nil &&
		req.ModelLimitsEnabled == nil &&
		req.ModelLimits == nil) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Status != nil && *req.Status != common.TokenStatusEnabled && *req.Status != common.TokenStatusDisabled {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	userID := c.GetInt("id")
	var tokens []model.Token
	if err := model.DB.Where("user_id = ? AND id IN ?", userID, ids).Find(&tokens).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	tokenByID := make(map[int]*model.Token, len(tokens))
	for i := range tokens {
		tokenByID[tokens[i].Id] = &tokens[i]
	}

	changedFields := make([]string, 0, 7)
	if req.Status != nil {
		changedFields = append(changedFields, "status")
	}
	if req.RemainQuota != nil || req.UnlimitedQuota != nil {
		changedFields = append(changedFields, "quota")
	}
	if req.Group != nil {
		changedFields = append(changedFields, "group")
	}
	if req.ExpiredTime != nil {
		changedFields = append(changedFields, "expired_time")
	}
	if req.ModelLimitsEnabled != nil || req.ModelLimits != nil {
		changedFields = append(changedFields, "model_limits")
	}

	result := batchUpdateResult{}
	for _, tokenID := range ids {
		token := tokenByID[tokenID]
		if token == nil {
			result.Failed = append(result.Failed, batchUpdateFailure{ID: tokenID, Message: "not_found"})
			continue
		}
		if err := applyBatchTokenChange(token, req); err != nil {
			result.Failed = append(result.Failed, batchUpdateFailure{ID: tokenID, Message: batchUpdateTokenFailureMessage(err)})
			continue
		}
		if err := token.Update(); err != nil {
			result.Failed = append(result.Failed, batchUpdateFailure{ID: tokenID, Message: "failed"})
			continue
		}
		result.Updated++
	}

	params := tokenBatchAuditParams(c, ids)
	params["updated"] = result.Updated
	params["failed"] = len(result.Failed)
	params["changed_fields"] = changedFields
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, result)
}
