package image

// ImageData encapsulates the sanitised image payload used by downstream consumers.
type ImageData struct {
	URL    string `json:"url,omitempty"`
	Data   string `json:"data,omitempty"`
	Format string `json:"format,omitempty"`
}

// ValidationResult captures the outcome of security validation.
type ValidationResult struct {
	IsValid      bool
	Format       string
	Width        int
	Height       int
	FileSize     int64
	Error        error
	SecurityRisk string
}

// Metrics aggregates pipeline statistics for observability.
type Metrics struct {
	TotalProcessed    int64
	URLDownloads      int64
	Base64Direct      int64
	FailedValidations int64
	SecurityIncidents int64
}
