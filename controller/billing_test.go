package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousDisplayTokenStatEnabled := common.DisplayTokenStatEnabled
	previousQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()

	common.RedisEnabled = false
	common.DisplayTokenStatEnabled = true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.DisplayTokenStatEnabled = previousDisplayTokenStatEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousQuotaDisplayType
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func performBillingRequest(t *testing.T, handler gin.HandlerFunc, userId int, tokenId int) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/dashboard/billing", nil)
	c.Set("id", userId)
	c.Set("token_id", tokenId)
	handler(c)
	return recorder
}

func TestBillingStatsUseUserQuotaForUnlimitedToken(t *testing.T) {
	db := setupBillingControllerTestDB(t)
	user := model.User{
		Username: "billing-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 900, UsedQuota: 100,
		AffCode: "billing-user-aff",
	}
	require.NoError(t, db.Create(&user).Error)

	limitedToken := model.Token{
		UserId: user.Id, Key: "billing-limited-token", Name: "limited",
		Status: common.TokenStatusEnabled, ExpiredTime: 1000, RemainQuota: 200, UsedQuota: 50,
	}
	unlimitedToken := model.Token{
		UserId: user.Id, Key: "billing-unlimited-token", Name: "unlimited",
		Status: common.TokenStatusEnabled, ExpiredTime: 2000, RemainQuota: 999999, UsedQuota: 888888,
		UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&limitedToken).Error)
	require.NoError(t, db.Create(&unlimitedToken).Error)

	tests := []struct {
		name       string
		token      model.Token
		wantTotal  float64
		wantUsed   float64
		wantRemain float64
	}{
		{name: "limited token", token: limitedToken, wantTotal: 250, wantUsed: 50, wantRemain: 200},
		{name: "unlimited token", token: unlimitedToken, wantTotal: 1000, wantUsed: 100, wantRemain: 900},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscriptionRecorder := performBillingRequest(t, GetSubscription, user.Id, tt.token.Id)
			assert.Equal(t, http.StatusOK, subscriptionRecorder.Code)
			var subscription OpenAISubscriptionResponse
			require.NoError(t, common.Unmarshal(subscriptionRecorder.Body.Bytes(), &subscription))
			assert.Equal(t, tt.wantTotal, subscription.HardLimitUSD)
			assert.Equal(t, tt.token.ExpiredTime, subscription.AccessUntil)

			usageRecorder := performBillingRequest(t, GetUsage, user.Id, tt.token.Id)
			assert.Equal(t, http.StatusOK, usageRecorder.Code)
			var usage OpenAIUsageResponse
			require.NoError(t, common.Unmarshal(usageRecorder.Body.Bytes(), &usage))
			assert.Equal(t, tt.wantUsed*100, usage.TotalUsage)
			assert.Equal(t, tt.wantRemain, subscription.HardLimitUSD-usage.TotalUsage/100)
		})
	}
}
