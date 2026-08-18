package relay

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingUsageFromResponseParseError(t *testing.T) {
	badBodyError := types.NewError(errors.New("response conversion failed"), types.ErrorCodeBadResponseBody)

	tests := []struct {
		name      string
		usage     any
		apiErr    *types.NewAPIError
		wantValid bool
	}{
		{
			name:      "parsed token usage",
			usage:     &dto.Usage{PromptTokens: 12},
			apiErr:    badBodyError,
			wantValid: true,
		},
		{
			name: "canonical billing usage",
			usage: &dto.Usage{
				BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{CompletionTokens: 7}),
			},
			apiErr:    badBodyError,
			wantValid: true,
		},
		{
			name:      "empty usage",
			usage:     &dto.Usage{},
			apiErr:    badBodyError,
			wantValid: false,
		},
		{
			name:      "nil usage",
			usage:     nil,
			apiErr:    badBodyError,
			wantValid: false,
		},
		{
			name:      "other error code",
			usage:     &dto.Usage{PromptTokens: 12},
			apiErr:    types.NewError(errors.New("upstream error"), types.ErrorCodeBadResponse),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, ok := billingUsageFromResponseParseError(tt.usage, tt.apiErr)
			assert.Equal(t, tt.wantValid, ok)
			if tt.wantValid {
				require.NotNil(t, actual)
			} else {
				assert.Nil(t, actual)
			}
		})
	}
}
