package ratio_setting

import "testing"

func TestFormatMatchingModelName_GPT5EffortSuffix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "gpt5 low", in: "gpt-5.2-low", out: "gpt-5.2"},
		{name: "gpt5 codex medium", in: "gpt-5.3-codex-medium", out: "gpt-5.3-codex"},
		{name: "gpt5 codex xhigh", in: "gpt-5.2-codex-xhigh", out: "gpt-5.2-codex"},
		{name: "gpt5 minimal", in: "gpt-5-minimal", out: "gpt-5"},
		{name: "non gpt5 unchanged", in: "claude-opus-4-6-low", out: "claude-opus-4-6-low"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatMatchingModelName(tc.in)
			if got != tc.out {
				t.Fatalf("FormatMatchingModelName(%q)=%q, want %q", tc.in, got, tc.out)
			}
		})
	}
}
