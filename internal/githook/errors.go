package githook

type sentinelError string

func (e sentinelError) Error() string {
	return string(e)
}

const (
	errMalformedInput sentinelError = "malformed pre-receive input"
	errInvalidRef     sentinelError = "invalid ref"
	errBranchDeletion sentinelError = "branch deletion is not allowed"
	errPathNotAllowed sentinelError = "path is not in the sync whitelist"
	errDisallowedMode sentinelError = "disallowed file mode"
	errSecretDetected sentinelError = "secret pattern detected in file content"
)
