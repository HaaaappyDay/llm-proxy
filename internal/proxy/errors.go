package proxy

type errorEnvelope struct {
	Error errorObject `json:"error"`
}

type errorObject struct {
	Type               string `json:"type"`
	Message            string `json:"message"`
	Status             int    `json:"status,omitempty"`
	SourceFormat       string `json:"source_format,omitempty"`
	TargetFormat       string `json:"target_format,omitempty"`
	UnsupportedFeature string `json:"unsupported_feature,omitempty"`
	UpstreamStatus     int    `json:"upstream_status,omitempty"`
	RetryAfter         string `json:"retry_after,omitempty"`
	BodyPreview        string `json:"body_preview,omitempty"`
	BodyTruncated      bool   `json:"body_truncated,omitempty"`
}

func newErrorEnvelope(errorType, message string) errorEnvelope {
	return errorEnvelope{
		Error: errorObject{
			Type:    errorType,
			Message: message,
		},
	}
}

func newStatusErrorEnvelope(status int, errorType, message string) errorEnvelope {
	out := newErrorEnvelope(errorType, message)
	out.Error.Status = status
	return out
}

type invalidRequestError struct {
	err error
}

func newInvalidRequestError(err error) error {
	return &invalidRequestError{err: err}
}

func (e *invalidRequestError) Error() string {
	return e.err.Error()
}

func (e *invalidRequestError) Unwrap() error {
	return e.err
}
