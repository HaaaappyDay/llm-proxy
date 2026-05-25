package proxy

type errorEnvelope struct {
	Error errorObject `json:"error"`
}

type errorObject struct {
	Type               string `json:"type"`
	Message            string `json:"message"`
	SourceFormat       string `json:"source_format,omitempty"`
	TargetFormat       string `json:"target_format,omitempty"`
	UnsupportedFeature string `json:"unsupported_feature,omitempty"`
	UpstreamStatus     int    `json:"upstream_status,omitempty"`
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
