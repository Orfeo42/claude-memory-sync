package guardrails

type guardrailError string

func (e guardrailError) Error() string {
	return string(e)
}

const (
	ErrInvalidFilename     guardrailError = "invalid entry filename"
	ErrIndexTarget         guardrailError = "memory.md is not a valid op target"
	ErrNoFrontmatter       guardrailError = "entry is missing YAML frontmatter"
	ErrMissingName         guardrailError = "frontmatter is missing a non-empty name field"
	ErrMissingDescription  guardrailError = "frontmatter is missing a non-empty description field"
	ErrMissingMetadataType guardrailError = "frontmatter metadata block is missing a non-empty type field"
	ErrSecretDetected      guardrailError = "secret pattern detected in content"
	ErrDigestTooLarge      guardrailError = "digest exceeds maximum allowed size"
)
