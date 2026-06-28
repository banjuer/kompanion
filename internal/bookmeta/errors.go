package bookmeta

import "errors"

var (
	ErrProviderDisabled = errors.New("metadata provider disabled")
	ErrNoCookie         = errors.New("metadata provider cookie is empty")
	ErrBookNotFound     = errors.New("book metadata not found")
)
