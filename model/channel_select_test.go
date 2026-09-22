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

func TestSelectSatisfiedChannelExhaustsPriorityBeforeDowngrade(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 101, "default", "test-model", 100, 80, true)
			createChannelSelectionTestChannel(t, 102, "default", "test-model", 100, 20, true)
			createChannelSelectionTestChannel(t, 103, "default", "test-model", 50, 100, true)
			InitChannelCache()

			attempted := make(map[int]struct{})
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "test-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   1,
				CurrentGroup:        "default",
			}

			first, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Contains(t, []int{101, 102}, first.Id)
			attempted[first.Id] = struct{}{}

			second, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Contains(t, []int{101, 102}, second.Id)
			assert.NotEqual(t, first.Id, second.Id)
			attempted[second.Id] = struct{}{}

			third, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, third)
			assert.Equal(t, 103, third.Id)
			attempted[third.Id] = struct{}{}

			options.LastChannelID = third.Id
			repeated, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, repeated)
			assert.Equal(t, 103, repeated.Id)
		})
	}
}

func TestSelectSatisfiedChannelRecalculatesCurrentPriorities(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 201, "default", "dynamic-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 202, "default", "dynamic-model", 50, 100, true)
			InitChannelCache()

			attempted := map[int]struct{}{201: {}, 202: {}}
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "dynamic-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   1,
				CurrentGroup:        "default",
			}
			channel, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 201, channel.Id)

			newLowestPriority := int64(10)
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 201).Update("priority", newLowestPriority).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 201).Update("priority", newLowestPriority).Error)
			InitChannelCache()
			channel, _, err = SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 202, channel.Id)
		})
	}
}

func TestSelectSatisfiedChannelUsesCurrentWeightsAndEnabledState(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 301, "default", "mutable-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 302, "default", "mutable-model", 100, 0, true)
			InitChannelCache()

			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "mutable-model",
				AttemptedChannelIDs: make(map[int]struct{}),
				RemainingAttempts:   1,
				CurrentGroup:        "default",
			}
			channel, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 301, channel.Id)

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 301).Update("weight", 0).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 301).Update("weight", 0).Error)
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 302).Update("weight", 100).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 302).Update("weight", 100).Error)
			InitChannelCache()
			channel, _, err = SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 302, channel.Id)

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 302).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 302).Update("enabled", false).Error)
			InitChannelCache()
			channel, _, err = SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 301, channel.Id)
		})
	}
}

func TestSelectSatisfiedChannelPreservesHighPriorityRetries(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 401, "default", "retry-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 402, "default", "retry-model", 50, 100, true)
			createChannelSelectionTestChannel(t, 403, "default", "retry-model", 25, 100, true)
			InitChannelCache()

			attempted := make(map[int]struct{})
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "retry-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   4,
				CurrentGroup:        "default",
			}

			first, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 401, first.Id)
			attempted[first.Id] = struct{}{}

			options.LastChannelID = first.Id
			options.RemainingAttempts = 3
			repeated, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, repeated)
			assert.Equal(t, 401, repeated.Id)

			options.RemainingAttempts = 2
			second, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, 402, second.Id)
			attempted[second.Id] = struct{}{}

			options.LastChannelID = second.Id
			options.RemainingAttempts = 1
			third, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, third)
			assert.Equal(t, 403, third.Id)
		})
	}
}

func TestSelectSatisfiedChannelUsesDistinctWhenEnoughChannels(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 501, "default", "distinct-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 502, "default", "distinct-model", 50, 100, true)
			createChannelSelectionTestChannel(t, 503, "default", "distinct-model", 25, 100, true)
			InitChannelCache()

			attempted := make(map[int]struct{})
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "distinct-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   3,
				CurrentGroup:        "default",
			}
			for _, expected := range []int{501, 502, 503} {
				channel, _, err := SelectSatisfiedChannel(options)
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, expected, channel.Id)
				attempted[channel.Id] = struct{}{}
				options.LastChannelID = channel.Id
				options.RemainingAttempts--
			}
		})
	}
}

func TestSelectSatisfiedChannelRepeatsLastCurrentPriority(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 601, "default", "repeat-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 602, "default", "repeat-model", 50, 100, true)
			InitChannelCache()

			attempted := map[int]struct{}{601: {}, 602: {}}
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "repeat-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   2,
				CurrentGroup:        "default",
				LastChannelID:       602,
			}
			channel, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 602, channel.Id)

			options.LastChannelID = 601
			channel, _, err = SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 601, channel.Id)
		})
	}
}

func TestSelectSatisfiedChannelReactsToCandidateLoss(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 701, "default", "dynamic-retry-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 702, "default", "dynamic-retry-model", 50, 100, true)
			createChannelSelectionTestChannel(t, 703, "default", "dynamic-retry-model", 25, 100, true)
			InitChannelCache()

			attempted := make(map[int]struct{})
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "dynamic-retry-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   4,
				CurrentGroup:        "default",
			}
			first, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 701, first.Id)
			attempted[first.Id] = struct{}{}

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 702).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 702).Update("enabled", false).Error)
			InitChannelCache()

			options.LastChannelID = first.Id
			options.RemainingAttempts = 3
			channel, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 701, channel.Id)
		})
	}
}

func TestSelectSatisfiedChannelReactsToCandidateGrowth(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 801, "default", "dynamic-growth-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 802, "default", "dynamic-growth-model", 50, 100, true)
			InitChannelCache()

			attempted := map[int]struct{}{801: {}}
			options := ChannelSelectionOptions{
				Groups:              []string{"default"},
				ModelName:           "dynamic-growth-model",
				AttemptedChannelIDs: attempted,
				RemainingAttempts:   2,
				CurrentGroup:        "default",
				LastChannelID:       801,
			}
			repeated, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, repeated)
			assert.Equal(t, 801, repeated.Id)

			createChannelSelectionTestChannel(t, 803, "default", "dynamic-growth-model", 200, 100, true)
			InitChannelCache()
			channel, _, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 803, channel.Id)
		})
	}
}

func TestSelectSatisfiedChannelHonorsForcedChannel(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelSelectionTest(t, memoryCacheEnabled)
			createChannelSelectionTestChannel(t, 901, "default", "forced-model", 100, 100, true)
			createChannelSelectionTestChannel(t, 902, "default", "forced-model", 50, 100, true)
			InitChannelCache()

			options := ChannelSelectionOptions{
				Groups:            []string{"default"},
				ModelName:         "forced-model",
				RemainingAttempts: 1,
				CurrentGroup:      "default",
				ForcedChannelID:   902,
			}
			channel, group, err := SelectSatisfiedChannel(options)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 902, channel.Id)
			assert.Equal(t, "default", group)

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 902).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 902).Update("enabled", false).Error)
			InitChannelCache()
			channel, _, err = SelectSatisfiedChannel(options)
			require.ErrorIs(t, err, ErrForcedChannelUnavailable)
			assert.Nil(t, channel)
		})
	}
}
