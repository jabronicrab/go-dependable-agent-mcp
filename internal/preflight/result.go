package preflight

import "time"

// Status is the overall readiness state of a completed dependency check.
type Status string

const (
	StatusReady    Status = "ready"
	StatusNotReady Status = "not_ready"
)

// StageName identifies one layer of the readiness check.
type StageName string

const (
	StageDNS  StageName = "dns"
	StageTCP  StageName = "tcp"
	StageTLS  StageName = "tls"
	StageHTTP StageName = "http"
)

// StageStatus describes whether a readiness stage ran and what it observed.
type StageStatus string

const (
	StagePassed        StageStatus = "passed"
	StageFailed        StageStatus = "failed"
	StageNotAttempted  StageStatus = "not_attempted"
	StageNotApplicable StageStatus = "not_applicable"
)

// StageReason explains why a readiness stage did not run.
type StageReason string

const (
	StageReasonPriorStageFailed      StageReason = "prior_stage_failed"
	StageReasonProtocolNotApplicable StageReason = "protocol_not_applicable"
	StageReasonLiteralIP             StageReason = "literal_ip"
)

// ErrorCategory is the stable caller-facing classification of a failure.
type ErrorCategory string

const (
	ErrorUnknownDependency       ErrorCategory = "unknown_dependency"
	ErrorDNSNotFound             ErrorCategory = "dns_not_found"
	ErrorDNSTimeout              ErrorCategory = "dns_timeout"
	ErrorDNSFailed               ErrorCategory = "dns_failed"
	ErrorConnectionRefused       ErrorCategory = "connection_refused"
	ErrorConnectionTimeout       ErrorCategory = "connection_timeout"
	ErrorConnectionFailed        ErrorCategory = "connection_failed"
	ErrorTLSCertificateExpired   ErrorCategory = "tls_certificate_expired"
	ErrorTLSHostnameMismatch     ErrorCategory = "tls_hostname_mismatch"
	ErrorTLSUntrustedCertificate ErrorCategory = "tls_untrusted_certificate"
	ErrorTLSTimeout              ErrorCategory = "tls_timeout"
	ErrorTLSHandshakeFailed      ErrorCategory = "tls_handshake_failed"
	ErrorHTTPUnexpectedStatus    ErrorCategory = "http_unexpected_status"
	ErrorHTTPTimeout             ErrorCategory = "http_timeout"
	ErrorHTTPRequestFailed       ErrorCategory = "http_request_failed"
	ErrorCancelled               ErrorCategory = "cancelled"
	ErrorInternal                ErrorCategory = "internal_error"
)

// Failure contains safe structured failure details without exposing raw OS errors.
type Failure struct {
	Category   ErrorCategory `json:"category"`
	Message    string        `json:"message"`
	HTTPStatus int           `json:"http_status,omitempty"`
}

// StageResult records the outcome and bounded duration of one readiness stage.
type StageResult struct {
	Name       StageName   `json:"name"`
	Status     StageStatus `json:"status"`
	DurationMS int64       `json:"duration_ms"`
	Reason     StageReason `json:"reason,omitempty"`
}

// Result is the complete domain result of checking one configured dependency.
type Result struct {
	Dependency  string        `json:"dependency"`
	Status      Status        `json:"status"`
	CheckedAt   time.Time     `json:"checked_at"`
	DurationMS  int64         `json:"duration_ms"`
	FailedStage StageName     `json:"failed_stage,omitempty"`
	Error       *Failure      `json:"error,omitempty"`
	Stages      []StageResult `json:"stages"`

	cause error
}

// Cause returns the internal diagnostic cause of a failed check. The value is
// deliberately excluded from JSON and must never be exposed directly to MCP callers.
func (r Result) Cause() error {
	return r.cause
}
