package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type batchUpdateUsersRequest struct {
	IDs             []int                       `json:"ids"`
	Status          *int                        `json:"status,omitempty"`
	Group           *string                     `json:"group,omitempty"`
	QuotaAdjustment *UserQuotaAdjustmentRequest `json:"quota_adjustment,omitempty"`
}

var errBatchCannotDisableRootUser = errors.New("cannot disable root user")

func applyBatchUserChange(userID int, req batchUpdateUsersRequest, operatorRole int) (*model.User, *model.UserQuotaAdjustment, error) {
	var user *model.User
	var adjustment *model.UserQuotaAdjustment
	var previousAuthVersion int64

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		current, err := model.GetUserByIdForUpdateInTx(tx, userID)
		if err != nil {
			return err
		}
		previousAuthVersion = current.AuthVersion
		if !canManageTargetRole(operatorRole, current.Role) {
			return errCannotManageTargetUser
		}
		if req.Status != nil && *req.Status == common.UserStatusDisabled && current.Role == common.RoleRootUser {
			return errBatchCannotDisableRootUser
		}

		if req.Status != nil {
			current.Status = *req.Status
		}
		if req.Group != nil {
			current.Group = *req.Group
		}
		if err := current.UpdateWithTx(tx, false); err != nil {
			return err
		}
		if req.QuotaAdjustment != nil {
			adjustment, err = model.AdjustUserQuotaInTx(
				tx,
				current,
				operatorRole,
				req.QuotaAdjustment.Mode,
				req.QuotaAdjustment.Value,
			)
			if err != nil {
				return err
			}
		}
		user = current
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if err := model.PublishUserAuthCache(user.Id); err != nil {
		return user, adjustment, err
	}
	if user.AuthVersion > previousAuthVersion {
		if _, err := model.RevokeAllUserSessions(user.Id, "admin_batch_user_update"); err != nil {
			return user, adjustment, err
		}
	}
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog("failed to invalidate tokens cache after batch user update: " + err.Error())
	}
	model.SyncUserQuotaAdjustmentCache(adjustment)
	return user, adjustment, nil
}

func batchUpdateUserFailureMessage(err error) string {
	switch {
	case errors.Is(err, errCannotManageTargetUser):
		return "permission_denied"
	case errors.Is(err, errBatchCannotDisableRootUser):
		return "root_protected"
	case errors.Is(err, model.ErrInvalidUserQuotaAdjustment):
		return "invalid_quota_adjustment"
	case errors.Is(err, model.ErrWalletQuotaLimitExceeded):
		return "quota_limit_exceeded"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "not_found"
	default:
		return "failed"
	}
}

// BatchUpdateUsers applies an allowlisted patch to multiple users.
func BatchUpdateUsers(c *gin.Context) {
	var req batchUpdateUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	ids, ok := normalizeBatchIDs(req.IDs)
	if !ok || (req.Status == nil && req.Group == nil && req.QuotaAdjustment == nil) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Status != nil && *req.Status != common.UserStatusEnabled && *req.Status != common.UserStatusDisabled {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.QuotaAdjustment != nil {
		mode := req.QuotaAdjustment.Mode
		value := req.QuotaAdjustment.Value
		if mode != "add" && mode != "subtract" && mode != "override" {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if mode != "override" && value <= 0 {
			common.ApiErrorI18n(c, i18n.MsgUserQuotaChangeZero)
			return
		}
		if value > common.MaxWalletQuota || value < -common.MaxWalletQuota {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}

	operatorRole := c.GetInt("role")
	result := batchUpdateResult{}
	for _, userID := range ids {
		_, adjustment, err := applyBatchUserChange(userID, req, operatorRole)
		if err != nil {
			message := batchUpdateUserFailureMessage(err)
			result.Failed = append(result.Failed, batchUpdateFailure{
				ID:      userID,
				Message: message,
			})
			if req.QuotaAdjustment != nil {
				recordUserQuotaAdjustmentFailure(c, ManageRequest{
					Id:    userID,
					Mode:  req.QuotaAdjustment.Mode,
					Value: req.QuotaAdjustment.Value,
				}, message)
			}
			continue
		}
		result.Updated++
		if adjustment != nil {
			recordUserQuotaAdjustmentSuccess(c, ManageRequest{
				Id:    userID,
				Mode:  req.QuotaAdjustment.Mode,
				Value: req.QuotaAdjustment.Value,
			}, adjustment)
		}
	}

	params := map[string]any{
		"count":              result.Updated,
		"total":              len(ids),
		"failed":             len(result.Failed),
		"requested_user_ids": ids[:min(len(ids), 100)],
	}
	if len(ids) > 100 {
		params["requested_user_ids_truncated"] = true
	}
	recordManageAudit(c, "user.batch_update", params)
	common.ApiSuccess(c, result)
}
