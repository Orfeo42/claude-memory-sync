package syncer

import "errors"

var (
	errInvalidNamespacePath = errors.New("invalid namespace path")
	errUnknownCanonicalKey  = errors.New("unknown canonical key")
	errRequestFailed        = errors.New("request failed")
	errSkipSymlinkedFile    = errors.New("skip symlinked file")
)
