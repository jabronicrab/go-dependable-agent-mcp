package preflight

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"runtime"
	"syscall"
)

const windowsWSAECONNREFUSED syscall.Errno = 10061

var errNoResolvedAddresses = errors.New("resolver returned no addresses")

type unexpectedHTTPStatusError struct {
	status   int
	accepted []int
}

func (e *unexpectedHTTPStatusError) Error() string {
	return fmt.Sprintf(
		"received HTTP status %d; accepted statuses are %v",
		e.status,
		e.accepted,
	)
}

func classifyDNS(
	ctx context.Context,
	err error,
) *Failure {
	if failure := classifyContext(
		ctx,
		err,
		ErrorDNSTimeout,
		"DNS lookup timed out",
	); failure != nil {
		return failure
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsNotFound:
			return &Failure{
				Category: ErrorDNSNotFound,
				Message:  "hostname could not be resolved",
			}

		case dnsErr.IsTimeout:
			return &Failure{
				Category: ErrorDNSTimeout,
				Message:  "DNS lookup timed out",
			}
		}
	}

	return &Failure{
		Category: ErrorDNSFailed,
		Message:  "DNS lookup failed",
	}
}

func classifyTCP(
	ctx context.Context,
	err error,
) *Failure {
	if failure := classifyContext(
		ctx,
		err,
		ErrorConnectionTimeout,
		"TCP connection timed out",
	); failure != nil {
		return failure
	}

	var attempts *dialAttemptsError
	if errors.As(err, &attempts) && len(attempts.errs) > 0 {
		allRefused := true
		allTimeout := true

		for _, attempt := range attempts.errs {
			allRefused = allRefused &&
				isConnectionRefused(attempt)

			allTimeout = allTimeout &&
				isTimeout(attempt)
		}

		switch {
		case allRefused:
			return &Failure{
				Category: ErrorConnectionRefused,
				Message:  "TCP connection was refused",
			}

		case allTimeout:
			return &Failure{
				Category: ErrorConnectionTimeout,
				Message:  "TCP connection timed out",
			}

		default:
			return &Failure{
				Category: ErrorConnectionFailed,
				Message:  "TCP connection failed",
			}
		}
	}

	if isConnectionRefused(err) {
		return &Failure{
			Category: ErrorConnectionRefused,
			Message:  "TCP connection was refused",
		}
	}

	if isTimeout(err) {
		return &Failure{
			Category: ErrorConnectionTimeout,
			Message:  "TCP connection timed out",
		}
	}

	return &Failure{
		Category: ErrorConnectionFailed,
		Message:  "TCP connection failed",
	}
}

func classifyTLS(
	ctx context.Context,
	err error,
) *Failure {
	if failure := classifyContext(
		ctx,
		err,
		ErrorTLSTimeout,
		"TLS handshake timed out",
	); failure != nil {
		return failure
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return &Failure{
			Category: ErrorTLSHostnameMismatch,
			Message:  "TLS certificate does not match the configured hostname",
		}
	}

	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return &Failure{
			Category: ErrorTLSUntrustedCertificate,
			Message:  "TLS certificate is not trusted",
		}
	}

	var invalidCertErr x509.CertificateInvalidError
	if errors.As(err, &invalidCertErr) &&
		invalidCertErr.Reason == x509.Expired {
		return &Failure{
			Category: ErrorTLSCertificateExpired,
			Message:  "TLS certificate is expired or not yet valid",
		}
	}

	if isTimeout(err) {
		return &Failure{
			Category: ErrorTLSTimeout,
			Message:  "TLS handshake timed out",
		}
	}

	return &Failure{
		Category: ErrorTLSHandshakeFailed,
		Message:  "TLS handshake failed",
	}
}

func classifyHTTP(
	ctx context.Context,
	err error,
) *Failure {
	if failure := classifyContext(
		ctx,
		err,
		ErrorHTTPTimeout,
		"HTTP request timed out",
	); failure != nil {
		return failure
	}

	var statusErr *unexpectedHTTPStatusError
	if errors.As(err, &statusErr) {
		return &Failure{
			Category:   ErrorHTTPUnexpectedStatus,
			Message:    statusErr.Error(),
			HTTPStatus: statusErr.status,
		}
	}

	if isTimeout(err) {
		return &Failure{
			Category: ErrorHTTPTimeout,
			Message:  "HTTP request timed out",
		}
	}

	return &Failure{
		Category: ErrorHTTPRequestFailed,
		Message:  "HTTP readiness request failed",
	}
}

func classifyContext(
	ctx context.Context,
	err error,
	timeoutCategory ErrorCategory,
	timeoutMessage string,
) *Failure {
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(ctx.Err(), context.Canceled):
		return &Failure{
			Category: ErrorCancelled,
			Message:  "dependency check was cancelled",
		}

	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &Failure{
			Category: timeoutCategory,
			Message:  timeoutMessage,
		}

	default:
		return nil
	}
}

func isConnectionRefused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	if runtime.GOOS != "windows" {
		return false
	}

	var errno syscall.Errno
	return errors.As(err, &errno) &&
		errno == windowsWSAECONNREFUSED
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) &&
		netErr.Timeout()
}
