package llm

import "claude-memory-sync/internal/domain"

type llmError string

func (e llmError) Error() string {
	return string(e)
}

func (e llmError) Is(target error) bool {
	return target == error(domain.ErrInternal)
}

const ErrAnthropicEnvSet llmError = "ANTHROPIC_API_KEY is set, refusing to bill API credits"
