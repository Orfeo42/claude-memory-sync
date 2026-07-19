package store

import "errors"

var (
	ErrInvalidPath      = errors.New("invalid path")
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrNotFound         = errors.New("file not found")
)
