package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
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

func TestPinnedTaskPluginChannelTypesUsesPinnedGenerationIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("legacy-select", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "legacy-select")
	assert.Equal(t, []int{constant.ChannelTypeKling}, types)
	assert.Equal(t, []string{"legacy-select"}, keys)
	types, keys = pinnedTaskPluginIdentities(c, "another-plugin")
	assert.Empty(t, types)
	assert.Empty(t, keys)
	types, keys = pinnedTaskPluginIdentities(nil, "legacy-select")
	assert.Empty(t, types)
	assert.Empty(t, keys)
}

func TestPinnedTaskPluginChannelTypesLeavesGenericChannelsKeyed(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("generic-select", constant.ChannelTypeTaskPlugin), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "generic-select")
	assert.Empty(t, types)
	assert.Equal(t, []string{"generic-select"}, keys)
}

func TestPinnedTaskPluginChannelTypesIncludesSharedEndpointProviders(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(channelSelectEndpointPluginSource("gemini-select", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(channelSelectEndpointPluginSource("vertex-select", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
	})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})

	AppendTaskPluginIdentityFilter(c, candidates[0].Plugin.Meta.Key)
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi}, filters[0].TaskPluginChannelTypes)
	assert.Equal(t, []string{"gemini-select", "vertex-select"}, filters[0].TaskPluginKeys)
}

func channelSelectTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectChannelTypesField(channelType int) string {
	if channelType <= 0 || channelType == constant.ChannelTypeTaskPlugin {
		return ""
	}
	return fmt.Sprintf("channelTypes: [%d],", channelType)
}

func TestPinnedTaskPluginChannelTypesIncludesCompatibleTypes(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectCompatiblePluginSource("sora-select", constant.ChannelTypeSora, constant.ChannelTypeOpenAI), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "sora-select")
	assert.Equal(t, []int{constant.ChannelTypeSora, constant.ChannelTypeOpenAI}, types)
	assert.Equal(t, []string{"sora-select"}, keys)
}

func channelSelectCompatiblePluginSource(key string, channelType, compatibleType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d, %d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType, compatibleType)
}

func TestSharedType61IdentityFilterContainsAllCandidateKeys(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		_, err := registry.Register(channelSelectEndpointPluginSource(key, 0), jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Candidates: candidates})
	AppendTaskPluginIdentityFilter(c, "alpha")
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, "alpha", filters[0].TaskPluginKey)
	assert.Equal(t, []string{"alpha", "beta"}, filters[0].TaskPluginKeys)
	assert.Empty(t, filters[0].TaskPluginChannelTypes)
}
