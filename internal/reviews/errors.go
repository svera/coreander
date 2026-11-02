package reviews

import "errors"

var (
	ErrInvalidDocumentLink = errors.New("invalid document link")
	ErrDocumentNotFound    = errors.New("document not found")
)
