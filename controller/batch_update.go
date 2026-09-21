package controller

type batchUpdateFailure struct {
	ID      int    `json:"id"`
	Message string `json:"message,omitempty"`
}

type batchUpdateResult struct {
	Updated int                  `json:"updated"`
	Failed  []batchUpdateFailure `json:"failed,omitempty"`
}

func normalizeBatchIDs(ids []int) ([]int, bool) {
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, false
	}

	seen := make(map[int]struct{}, len(ids))
	normalized := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, false
		}
		if _, ok := seen[id]; ok {
			return nil, false
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, true
}
