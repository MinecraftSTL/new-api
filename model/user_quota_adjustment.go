package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrInvalidUserQuotaAdjustment = errors.New("invalid user quota adjustment")
	ErrUserQuotaPermission        = errors.New("cannot adjust quota for this user role")
)

// UserQuotaAdjustment is the immutable database snapshot of a committed manual
// adjustment. Pending relay deductions in the quota cache are not part of it.
type UserQuotaAdjustment struct {
	UserID   int
	Username string
	Before   int
	After    int
}

func validateUserQuotaAdjustment(mode string, value int) error {
	if mode != "add" && mode != "subtract" && mode != "override" {
		return ErrInvalidUserQuotaAdjustment
	}
	if mode != "override" && value <= 0 {
		return ErrInvalidUserQuotaAdjustment
	}
	if value > common.MaxWalletQuota || value < -common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	return nil
}

// AdjustUserQuotaInTx applies one quota adjustment to a user already locked by
// the caller's transaction. It does not publish cache changes; callers must do
// that only after the transaction commits.
func AdjustUserQuotaInTx(tx *gorm.DB, user *User, operatorRole int, mode string, value int) (*UserQuotaAdjustment, error) {
	if user == nil || user.Id <= 0 {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if err := validateUserQuotaAdjustment(mode, value); err != nil {
		return nil, err
	}
	if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
		return nil, ErrUserQuotaPermission
	}
	if user.Quota > common.MaxWalletQuota || user.Quota < -common.MaxWalletQuota {
		return nil, ErrWalletQuotaLimitExceeded
	}

	quota := decimal.NewFromInt(int64(value))
	switch mode {
	case "add":
		quota = decimal.NewFromInt(int64(user.Quota)).Add(quota)
	case "subtract":
		quota = decimal.NewFromInt(int64(user.Quota)).Sub(quota)
	}
	after, err := common.WalletQuotaFromDecimalStrict(quota)
	if err != nil {
		return nil, ErrWalletQuotaLimitExceeded
	}
	// An unchanged override is a successful operation, including on MySQL
	// configurations that count only changed rows in RowsAffected.
	if after != user.Quota {
		result := tx.Model(&User{}).Where("id = ?", user.Id).Update("quota", after)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, gorm.ErrRecordNotFound
		}
	}
	adjustment := &UserQuotaAdjustment{
		UserID:   user.Id,
		Username: user.Username,
		Before:   user.Quota,
		After:    after,
	}
	user.Quota = after
	return adjustment, nil
}

func AdjustUserQuota(userID, operatorRole int, mode string, value int) (*UserQuotaAdjustment, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserQuotaAdjustment
	}

	var adjustment *UserQuotaAdjustment
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		var err error
		adjustment, err = AdjustUserQuotaInTx(tx, &user, operatorRole, mode, value)
		return err
	})
	if err != nil {
		return nil, err
	}

	SyncUserQuotaAdjustmentCache(adjustment)
	return adjustment, nil
}

// SyncUserQuotaAdjustmentCache applies only the committed difference, preserving
// outstanding quota reservations.
func SyncUserQuotaAdjustmentCache(adjustment *UserQuotaAdjustment) {
	if adjustment == nil {
		return
	}
	delta := int64(adjustment.After) - int64(adjustment.Before)
	if delta != 0 {
		if err := cacheIncrUserQuota(adjustment.UserID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync manual quota adjustment for user %d: %s", adjustment.UserID, err))
		}
	}
}
