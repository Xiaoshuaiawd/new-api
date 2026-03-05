package codex

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequest_ReasoningSuffix(t *testing.T) {
	a := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	out, err := a.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.2-codex-xhigh",
	})
	require.NoError(t, err)

	req, ok := out.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	require.Equal(t, "gpt-5.2-codex", req.Model)
	require.NotNil(t, req.Reasoning)
	require.Equal(t, "xhigh", req.Reasoning.Effort)
	require.Equal(t, "auto", req.Reasoning.Summary)
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequest_ReasoningSuffixOverridesExistingReasoning(t *testing.T) {
	a := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	out, err := a.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.1-codex-low",
		Reasoning: &dto.Reasoning{
			Effort:  "high",
			Summary: "detailed",
		},
	})
	require.NoError(t, err)

	req, ok := out.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	require.Equal(t, "gpt-5.1-codex", req.Model)
	require.NotNil(t, req.Reasoning)
	require.Equal(t, "low", req.Reasoning.Effort)
	require.Equal(t, "auto", req.Reasoning.Summary)
	require.Equal(t, "low", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequest_NoSuffixKeepsReasoning(t *testing.T) {
	a := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	out, err := a.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.1-codex",
		Reasoning: &dto.Reasoning{
			Effort:  "medium",
			Summary: "detailed",
		},
	})
	require.NoError(t, err)

	req, ok := out.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	require.Equal(t, "gpt-5.1-codex", req.Model)
	require.NotNil(t, req.Reasoning)
	require.Equal(t, "medium", req.Reasoning.Effort)
	require.Equal(t, "detailed", req.Reasoning.Summary)
	require.Equal(t, "medium", info.ReasoningEffort)
}
