package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type ChannelSelectionOptions struct {
	Groups              []string
	ModelName           string
	AttemptedChannelIDs map[int]struct{}
	Filters             []dto.ChannelFilter
	RemainingAttempts   int
	CurrentGroup        string
	LastChannelID       int
}

type channelSelectionCandidate struct {
	channelID int
	group     string
	priority  int64
	weight    int
}

func SelectSatisfiedChannel(options ChannelSelectionOptions) (*Channel, string, error) {
	fallbackGroup := options.CurrentGroup
	if fallbackGroup == "" && len(options.Groups) > 0 {
		fallbackGroup = options.Groups[0]
	}
	if len(options.Groups) <= 0 || options.RemainingAttempts <= 0 {
		return nil, fallbackGroup, nil
	}

	candidates, err := loadChannelSelectionCandidates(options.Groups, options.ModelName, options.Filters)
	if err != nil {
		return nil, fallbackGroup, err
	}
	if len(candidates) <= 0 {
		return nil, fallbackGroup, nil
	}

	untriedCount := countUntriedChannelCandidates(candidates, options.AttemptedChannelIDs)
	if options.RemainingAttempts > untriedCount {
		if selected, ok := selectRepeatChannelCandidate(candidates, options.AttemptedChannelIDs, options.CurrentGroup, options.LastChannelID); ok {
			channel, err := CacheGetChannel(selected.channelID)
			return channel, selected.group, err
		}
	}

	selected, ok := selectDistinctChannelCandidate(candidates, options.AttemptedChannelIDs)
	if !ok {
		return nil, fallbackGroup, nil
	}
	channel, err := CacheGetChannel(selected.channelID)
	return channel, selected.group, err
}

func loadChannelSelectionCandidates(groups []string, modelName string, filters []dto.ChannelFilter) ([]channelSelectionCandidate, error) {
	if common.MemoryCacheEnabled {
		return loadCachedChannelSelectionCandidates(groups, modelName, filters)
	}
	return loadDatabaseChannelSelectionCandidates(groups, modelName, filters)
}

func countUntriedChannelCandidates(candidates []channelSelectionCandidate, attemptedChannelIDs map[int]struct{}) int {
	seen := make(map[int]struct{}, len(candidates))
	count := 0
	for _, candidate := range candidates {
		if _, attempted := attemptedChannelIDs[candidate.channelID]; attempted {
			continue
		}
		if _, duplicate := seen[candidate.channelID]; duplicate {
			continue
		}
		seen[candidate.channelID] = struct{}{}
		count++
	}
	return count
}

func selectDistinctChannelCandidate(candidates []channelSelectionCandidate, attemptedChannelIDs map[int]struct{}) (channelSelectionCandidate, bool) {
	for index, candidate := range candidates {
		if index > 0 && candidate.group == candidates[index-1].group {
			continue
		}
		selected, ok := selectDistinctChannelCandidateInGroup(candidates, candidate.group, attemptedChannelIDs)
		if ok {
			return selected, true
		}
	}
	return channelSelectionCandidate{}, false
}

func selectDistinctChannelCandidateInGroup(candidates []channelSelectionCandidate, group string, attemptedChannelIDs map[int]struct{}) (channelSelectionCandidate, bool) {
	var targetPriority int64
	hasTarget := false
	for _, candidate := range candidates {
		if candidate.group != group {
			continue
		}
		if _, attempted := attemptedChannelIDs[candidate.channelID]; attempted {
			continue
		}
		if !hasTarget || candidate.priority > targetPriority {
			targetPriority = candidate.priority
			hasTarget = true
		}
	}
	if !hasTarget {
		return channelSelectionCandidate{}, false
	}

	targets := make([]channelSelectionCandidate, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.group != group || candidate.priority != targetPriority {
			continue
		}
		if _, attempted := attemptedChannelIDs[candidate.channelID]; attempted {
			continue
		}
		if _, duplicate := seen[candidate.channelID]; duplicate {
			continue
		}
		seen[candidate.channelID] = struct{}{}
		targets = append(targets, candidate)
	}
	return pickWeightedChannelSelectionCandidate(targets)
}

func selectRepeatChannelCandidate(candidates []channelSelectionCandidate, attemptedChannelIDs map[int]struct{}, currentGroup string, lastChannelID int) (channelSelectionCandidate, bool) {
	targetGroup := ""
	if currentGroup != "" {
		for _, candidate := range candidates {
			if candidate.group != currentGroup || !channelWasAttempted(attemptedChannelIDs, candidate.channelID) {
				continue
			}
			targetGroup = currentGroup
			break
		}
	}
	if targetGroup == "" {
		for _, candidate := range candidates {
			if !channelWasAttempted(attemptedChannelIDs, candidate.channelID) {
				continue
			}
			targetGroup = candidate.group
			break
		}
	}
	if targetGroup == "" {
		return channelSelectionCandidate{}, false
	}

	var targetPriority int64
	hasTarget := false
	for _, candidate := range candidates {
		if candidate.group != targetGroup || candidate.channelID != lastChannelID {
			continue
		}
		if !channelWasAttempted(attemptedChannelIDs, candidate.channelID) {
			continue
		}
		targetPriority = candidate.priority
		hasTarget = true
		break
	}
	if !hasTarget {
		for _, candidate := range candidates {
			if candidate.group != targetGroup || !channelWasAttempted(attemptedChannelIDs, candidate.channelID) {
				continue
			}
			if !hasTarget || candidate.priority > targetPriority {
				targetPriority = candidate.priority
				hasTarget = true
			}
		}
	}
	if !hasTarget {
		return channelSelectionCandidate{}, false
	}

	targets := make([]channelSelectionCandidate, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.group != targetGroup || candidate.priority != targetPriority {
			continue
		}
		if !channelWasAttempted(attemptedChannelIDs, candidate.channelID) {
			continue
		}
		if _, duplicate := seen[candidate.channelID]; duplicate {
			continue
		}
		seen[candidate.channelID] = struct{}{}
		targets = append(targets, candidate)
	}
	return pickWeightedChannelSelectionCandidate(targets)
}

func channelWasAttempted(attemptedChannelIDs map[int]struct{}, channelID int) bool {
	_, attempted := attemptedChannelIDs[channelID]
	return attempted
}

func pickWeightedChannelSelectionCandidate(candidates []channelSelectionCandidate) (channelSelectionCandidate, bool) {
	if len(candidates) <= 0 {
		return channelSelectionCandidate{}, false
	}
	totalWeight := 0
	for _, candidate := range candidates {
		if candidate.weight > 0 {
			totalWeight += candidate.weight
		}
	}
	if totalWeight <= 0 {
		return candidates[common.GetRandomInt(len(candidates))], true
	}

	randomWeight := common.GetRandomInt(totalWeight)
	for _, candidate := range candidates {
		if candidate.weight <= 0 {
			continue
		}
		randomWeight -= candidate.weight
		if randomWeight < 0 {
			return candidate, true
		}
	}
	return candidates[len(candidates)-1], true
}

// selectChannelCandidateID chooses from the highest-priority layer that still
// has an untried channel. If all current candidates have been tried, callers may
// allow a repeat from the current lowest-priority layer.
func selectChannelCandidateID(candidates []channelSelectionCandidate, attemptedChannelIDs map[int]struct{}, allowLowestPriorityRepeat bool) (int, bool) {
	if len(candidates) == 0 {
		return 0, false
	}

	var targetPriority int64
	hasUntried := false
	lowestPriority := candidates[0].priority
	for _, candidate := range candidates {
		if candidate.priority < lowestPriority {
			lowestPriority = candidate.priority
		}
		if _, attempted := attemptedChannelIDs[candidate.channelID]; attempted {
			continue
		}
		if !hasUntried || candidate.priority > targetPriority {
			targetPriority = candidate.priority
			hasUntried = true
		}
	}

	if !hasUntried {
		if !allowLowestPriorityRepeat {
			return 0, false
		}
		targetPriority = lowestPriority
	}

	targets := make([]channelSelectionCandidate, 0, len(candidates))
	totalWeight := 0
	for _, candidate := range candidates {
		if candidate.priority != targetPriority {
			continue
		}
		if hasUntried {
			if _, attempted := attemptedChannelIDs[candidate.channelID]; attempted {
				continue
			}
		}
		targets = append(targets, candidate)
		totalWeight += candidate.weight
	}

	if len(targets) == 0 {
		return 0, false
	}
	if totalWeight <= 0 {
		return targets[common.GetRandomInt(len(targets))].channelID, true
	}

	randomWeight := common.GetRandomInt(totalWeight)
	for _, candidate := range targets {
		randomWeight -= candidate.weight
		if randomWeight < 0 {
			return candidate.channelID, true
		}
	}
	return targets[len(targets)-1].channelID, true
}
