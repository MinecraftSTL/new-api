package model

import "github.com/QuantumNous/new-api/common"

type channelSelectionCandidate struct {
	channelID int
	priority  int64
	weight    int
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
