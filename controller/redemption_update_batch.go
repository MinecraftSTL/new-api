package controller

import (
	"net/http"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type batchUpdateRedemptionsRequest struct {
	IDs         []int   `json:"ids"`
	Status      *int    `json:"status,omitempty"`
	Name        *string `json:"name,omitempty"`
	Quota       *int    `json:"quota,omitempty"`
	ExpiredTime *int64  `json:"expired_time,omitempty"`
}

// UpdateRedemptionBatch applies an allowlisted patch to multiple redemption codes.
func UpdateRedemptionBatch(c *gin.Context) {
	var req batchUpdateRedemptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	ids, ok := normalizeBatchIDs(req.IDs)
	if !ok || (req.Status == nil && req.Name == nil && req.Quota == nil && req.ExpiredTime == nil) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Status != nil && *req.Status != common.RedemptionCodeStatusEnabled && *req.Status != common.RedemptionCodeStatusDisabled {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Name != nil {
		nameLength := utf8.RuneCountInString(*req.Name)
		if nameLength == 0 || nameLength > 20 {
			common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
			return
		}
	}
	if req.Quota != nil {
		if *req.Quota <= 0 {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if err := common.ValidateWalletQuota(*req.Quota); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if req.ExpiredTime != nil {
		if valid, msg := validateExpiredTime(c, *req.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
	}

	var redemptions []model.Redemption
	if err := model.DB.Where("id IN ?", ids).Find(&redemptions).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	redemptionByID := make(map[int]*model.Redemption, len(redemptions))
	for i := range redemptions {
		redemptionByID[redemptions[i].Id] = &redemptions[i]
	}

	result := batchUpdateResult{}
	for _, redemptionID := range ids {
		redemption := redemptionByID[redemptionID]
		if redemption == nil {
			result.Failed = append(result.Failed, batchUpdateFailure{ID: redemptionID, Message: "not_found"})
			continue
		}
		if req.Status != nil {
			redemption.Status = *req.Status
		}
		if req.Name != nil {
			redemption.Name = *req.Name
		}
		if req.Quota != nil {
			redemption.Quota = *req.Quota
		}
		if req.ExpiredTime != nil {
			redemption.ExpiredTime = *req.ExpiredTime
		}
		if err := redemption.Update(); err != nil {
			result.Failed = append(result.Failed, batchUpdateFailure{ID: redemptionID, Message: "failed"})
			continue
		}
		result.Updated++
	}

	recordManageAudit(c, "redemption.update_batch", map[string]any{
		"count":     result.Updated,
		"total":     len(ids),
		"failed":    len(result.Failed),
		"code_ids":  ids[:min(len(ids), 100)],
		"truncated": len(ids) > 100,
	})
	common.ApiSuccess(c, result)
}
