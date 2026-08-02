package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetAutoGroupChannelSelectionTest(t *testing.T) {
	t.Helper()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	common.MemoryCacheEnabled = true
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	model.InitChannelCache()
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
		require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		model.InitChannelCache()
	})
}

func createAutoGroupChannelSelectionTestChannel(t *testing.T, channelID int, groups string, priority int64) {
	t.Helper()
	weight := uint(100)
	channel := &model.Channel{
		Id:       channelID,
		Type:     1,
		Key:      fmt.Sprintf("key-%d", channelID),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", channelID),
		Models:   "auto-model",
		Group:    groups,
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	for _, group := range strings.Split(groups, ",") {
		require.NoError(t, model.DB.Create(&model.Ability{
			Group:     group,
			Model:     "auto-model",
			ChannelId: channelID,
			Enabled:   true,
			Priority:  &priority,
			Weight:    weight,
		}).Error)
	}
}

func newAutoGroupChannelSelectionContext(crossGroupRetry bool, attemptedChannelIDs ...string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, crossGroupRetry)
	ctx.Set("use_channel", attemptedChannelIDs)
	return ctx
}

func TestCacheGetRandomSatisfiedChannelUsesGlobalAttemptsAcrossAutoGroups(t *testing.T) {
	resetAutoGroupChannelSelectionTest(t)
	createAutoGroupChannelSelectionTestChannel(t, 301, "default,vip", 100)
	createAutoGroupChannelSelectionTestChannel(t, 302, "default", 50)
	createAutoGroupChannelSelectionTestChannel(t, 303, "vip", 50)
	model.InitChannelCache()

	ctx := newAutoGroupChannelSelectionContext(true, "301")
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 302, channel.Id)
	assert.Equal(t, "default", selectedGroup)

	ctx.Set("use_channel", []string{"301", "302"})
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 303, channel.Id)
	assert.Equal(t, "vip", selectedGroup)

	ctx.Set("use_channel", []string{"301", "302", "303"})
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 303, channel.Id)
	assert.Equal(t, "vip", selectedGroup)

	createAutoGroupChannelSelectionTestChannel(t, 304, "default", 200)
	model.InitChannelCache()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 304, channel.Id)
	assert.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelStaysInCurrentAutoGroupWhenCrossGroupRetryDisabled(t *testing.T) {
	resetAutoGroupChannelSelectionTest(t)
	createAutoGroupChannelSelectionTestChannel(t, 401, "default,vip", 100)
	createAutoGroupChannelSelectionTestChannel(t, 402, "default", 50)
	createAutoGroupChannelSelectionTestChannel(t, 403, "vip", 200)
	model.InitChannelCache()

	ctx := newAutoGroupChannelSelectionContext(false, "401", "402")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "default")
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 402, channel.Id)
	assert.Equal(t, "default", selectedGroup)

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ?", []int{401, 402}).Update("status", common.ChannelStatusManuallyDisabled).Error)
	model.InitChannelCache()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-model",
		RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	assert.Nil(t, channel)
	assert.Equal(t, "default", selectedGroup)
}
