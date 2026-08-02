package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetChannelSelectionTest(t *testing.T, memoryCacheEnabled bool) {
	t.Helper()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = memoryCacheEnabled
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	InitChannelCache()
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
		require.NoError(t, DB.Exec("DELETE FROM channels").Error)
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		InitChannelCache()
	})
}

func createChannelSelectionTestChannel(t *testing.T, channelID int, groups string, modelName string, priority int64, weight uint, enabled bool) {
	t.Helper()
	status := common.ChannelStatusManuallyDisabled
	if enabled {
		status = common.ChannelStatusEnabled
	}
	channel := &Channel{
		Id:       channelID,
		Type:     1,
		Key:      fmt.Sprintf("key-%d", channelID),
		Status:   status,
		Name:     fmt.Sprintf("channel-%d", channelID),
		Models:   modelName,
		Group:    groups,
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, DB.Create(channel).Error)
	for _, group := range strings.Split(groups, ",") {
		require.NoError(t, DB.Create(&Ability{
			Group:     group,
			Model:     modelName,
			ChannelId: channelID,
			Enabled:   enabled,
			Priority:  &priority,
			Weight:    weight,
		}).Error)
	}
}

func TestGetRandomSatisfiedChannelExhaustsPriorityBeforeDowngrade(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 101, "default", "test-model", 100, 80, true)
			createChannelSelectionTestChannel(t, 102, "default", "test-model", 100, 20, true)
			createChannelSelectionTestChannel(t, 103, "default", "test-model", 50, 100, true)
			InitChannelCache()

			attempted := make(map[int]struct{})
			first, err := GetRandomSatisfiedChannel("default", "test-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Contains(t, []int{101, 102}, first.Id)
			attempted[first.Id] = struct{}{}

			second, err := GetRandomSatisfiedChannel("default", "test-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Contains(t, []int{101, 102}, second.Id)
			assert.NotEqual(t, first.Id, second.Id)
			attempted[second.Id] = struct{}{}

			third, err := GetRandomSatisfiedChannel("default", "test-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, third)
			assert.Equal(t, 103, third.Id)
			attempted[third.Id] = struct{}{}

			exhausted, err := GetRandomSatisfiedChannel("default", "test-model", attempted, "", false)
			require.NoError(t, err)
			assert.Nil(t, exhausted)

			repeated, err := GetRandomSatisfiedChannel("default", "test-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, repeated)
			assert.Equal(t, 103, repeated.Id)
		})
	}
}

func TestGetRandomSatisfiedChannelRecalculatesCurrentPriorities(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 201, "default", "dynamic-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 202, "default", "dynamic-model", 50, 100, true)
			InitChannelCache()

			attempted := map[int]struct{}{201: {}, 202: {}}
			channel, err := GetRandomSatisfiedChannel("default", "dynamic-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 202, channel.Id)

			createChannelSelectionTestChannel(t, 203, "default", "dynamic-model", 200, 100, true)
			InitChannelCache()
			channel, err = GetRandomSatisfiedChannel("default", "dynamic-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 203, channel.Id)

			attempted[203] = struct{}{}
			newLowestPriority := int64(10)
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 201).Update("priority", newLowestPriority).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 201).Update("priority", newLowestPriority).Error)
			InitChannelCache()
			channel, err = GetRandomSatisfiedChannel("default", "dynamic-model", attempted, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 201, channel.Id)
		})
	}
}

func TestGetRandomSatisfiedChannelUsesCurrentWeightsAndEnabledState(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 301, "default", "mutable-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 302, "default", "mutable-model", 100, 0, true)
			InitChannelCache()

			channel, err := GetRandomSatisfiedChannel("default", "mutable-model", nil, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 301, channel.Id)

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 301).Update("weight", 0).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 301).Update("weight", 0).Error)
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 302).Update("weight", 100).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 302).Update("weight", 100).Error)
			InitChannelCache()
			channel, err = GetRandomSatisfiedChannel("default", "mutable-model", nil, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 302, channel.Id)

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 302).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 302).Update("enabled", false).Error)
			InitChannelCache()
			channel, err = GetRandomSatisfiedChannel("default", "mutable-model", nil, "", true)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 301, channel.Id)
		})
	}
}
