package preflight

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
)

var testTimeouts = catalog.Timeouts{
	Total: 2 * time.Second,
	DNS:   500 * time.Millisecond,
	TCP:   500 * time.Millisecond,
	TLS:   500 * time.Millisecond,
	HTTP:  500 * time.Millisecond,
}

type fakeResolver struct {
	addresses []string
	err       error
	calls     int
}

func (r *fakeResolver) LookupHost(
	context.Context,
	string,
) ([]string, error) {
	r.calls++

	return r.addresses, r.err
}

type fakeDialer struct {
	conn  net.Conn
	err   error
	calls int
}

func (d *fakeDialer) DialContext(
	context.Context,
	string,
	string,
) (net.Conn, error) {
	d.calls++

	return d.conn, d.err
}

func TestCheckerHTTPReady(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.WriteHeader(
					http.StatusNoContent,
				)
			},
		),
	)
	defer server.Close()

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTP,
		[]int{http.StatusNoContent},
	)

	result := NewChecker().Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	assertReady(t, result)

	assertStage(
		t,
		result,
		StageDNS,
		StageNotApplicable,
	)

	assertStage(
		t,
		result,
		StageTCP,
		StagePassed,
	)

	assertStage(
		t,
		result,
		StageTLS,
		StageNotApplicable,
	)

	assertStage(
		t,
		result,
		StageHTTP,
		StagePassed,
	)
}

func TestCheckerHTTPUnexpectedStatusIsDomainFailure(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.WriteHeader(
					http.StatusServiceUnavailable,
				)
			},
		),
	)
	defer server.Close()

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTP,
		[]int{http.StatusOK},
	)

	result := NewChecker().Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.Status != StatusNotReady ||
		result.FailedStage != StageHTTP {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}

	if result.Error == nil ||
		result.Error.Category !=
			ErrorHTTPUnexpectedStatus ||
		result.Error.HTTPStatus !=
			http.StatusServiceUnavailable {
		t.Fatalf(
			"result error = %#v",
			result.Error,
		)
	}
}

func TestCheckerDoesNotFollowRedirects(
	t *testing.T,
) {
	var redirected atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/ready",
		func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.Header().Set(
				"Location",
				"/target",
			)

			w.WriteHeader(
				http.StatusFound,
			)
		},
	)

	mux.HandleFunc(
		"/target",
		func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			redirected.Add(1)

			w.WriteHeader(
				http.StatusOK,
			)
		},
	)

	server := httptest.NewServer(mux)
	defer server.Close()

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTP,
		[]int{http.StatusOK},
	)

	result := NewChecker().Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.Error == nil ||
		result.Error.Category !=
			ErrorHTTPUnexpectedStatus ||
		result.Error.HTTPStatus !=
			http.StatusFound {
		t.Fatalf(
			"result error = %#v",
			result.Error,
		)
	}

	if redirected.Load() != 0 {
		t.Fatalf(
			"redirect target calls = %d, want 0",
			redirected.Load(),
		)
	}
}

func TestCheckerHTTPSReadyWithControlledTrustRoot(
	t *testing.T,
) {
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.WriteHeader(
					http.StatusOK,
				)
			},
		),
	)

	server.Config.ErrorLog = log.New(
		io.Discard,
		"",
		0,
	)

	server.StartTLS()
	defer server.Close()

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())

	checker := NewChecker()
	checker.rootCAs = pool

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTPS,
		[]int{http.StatusOK},
	)

	result := checker.Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	assertReady(t, result)

	assertStage(
		t,
		result,
		StageTLS,
		StagePassed,
	)

	assertStage(
		t,
		result,
		StageHTTP,
		StagePassed,
	)
}

func TestCheckerHTTPSRejectsUntrustedCertificate(
	t *testing.T,
) {
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.WriteHeader(
					http.StatusOK,
				)
			},
		),
	)

	server.Config.ErrorLog = log.New(
		io.Discard,
		"",
		0,
	)

	server.StartTLS()
	defer server.Close()

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTPS,
		[]int{http.StatusOK},
	)

	result := NewChecker().Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.FailedStage != StageTLS ||
		result.Error == nil ||
		result.Error.Category !=
			ErrorTLSUntrustedCertificate {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}

	assertStage(
		t,
		result,
		StageHTTP,
		StageNotAttempted,
	)
}

func TestCheckerDNSNotFoundStopsBeforeTCP(
	t *testing.T,
) {
	resolver := &fakeResolver{
		err: &net.DNSError{
			Err:        "no such host",
			Name:       "missing.example",
			IsNotFound: true,
		},
	}

	dialer := &fakeDialer{}

	checker := &Checker{
		resolver: resolver,
		dialer:   dialer,
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "missing",
		Protocol: catalog.ProtocolTCP,
		Host:     "missing.example",
		Port:     443,
	}

	result := checker.Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.FailedStage != StageDNS ||
		result.Error == nil ||
		result.Error.Category !=
			ErrorDNSNotFound {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}

	if dialer.calls != 0 {
		t.Fatalf(
			"dialer calls = %d, want 0",
			dialer.calls,
		)
	}

	assertStage(
		t,
		result,
		StageTCP,
		StageNotAttempted,
	)
}

func TestCheckerClassifiesLocalConnectionRefused(
	t *testing.T,
) {
	listener, err := net.Listen(
		"tcp",
		"127.0.0.1:0",
	)

	if err != nil {
		t.Fatalf(
			"net.Listen() error = %v",
			err,
		)
	}

	address := listener.Addr().(*net.TCPAddr)
	port := uint16(address.Port)

	if err := listener.Close(); err != nil {
		t.Fatalf(
			"listener.Close() error = %v",
			err,
		)
	}

	dependency := catalog.Dependency{
		Name:     "closed-port",
		Protocol: catalog.ProtocolTCP,
		Host:     "127.0.0.1",
		Port:     port,
	}

	result := NewChecker().Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.FailedStage != StageTCP ||
		result.Error == nil ||
		result.Error.Category !=
			ErrorConnectionRefused {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}
}

func TestCheckerClassifiesConnectionRefused(
	t *testing.T,
) {
	resolver := &fakeResolver{
		addresses: []string{
			"127.0.0.1",
		},
	}

	dialer := &fakeDialer{
		err: syscall.ECONNREFUSED,
	}

	checker := &Checker{
		resolver: resolver,
		dialer:   dialer,
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolTCP,
		Host:     "api.example",
		Port:     443,
	}

	result := checker.Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.FailedStage != StageTCP ||
		result.Error == nil ||
		result.Error.Category !=
			ErrorConnectionRefused {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}
}

func TestCheckerClassifiesDNSTimeoutWithoutSleeping(
	t *testing.T,
) {
	resolver := &fakeResolver{
		err: context.DeadlineExceeded,
	}

	checker := &Checker{
		resolver: resolver,
		dialer:   &fakeDialer{},
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolTCP,
		Host:     "api.example",
		Port:     443,
	}

	result := checker.Check(
		context.Background(),
		dependency,
		testTimeouts,
	)

	if result.FailedStage != StageDNS ||
		result.Error == nil ||
		result.Error.Category !=
			ErrorDNSTimeout {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}
}

func TestCheckerHonorsCallerCancellation(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	cancel()

	resolver := &fakeResolver{
		err: context.Canceled,
	}

	checker := &Checker{
		resolver: resolver,
		dialer:   &fakeDialer{},
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolTCP,
		Host:     "api.example",
		Port:     443,
	}

	result := checker.Check(
		ctx,
		dependency,
		testTimeouts,
	)

	if result.Error == nil ||
		result.Error.Category != ErrorCancelled {
		t.Fatalf(
			"result = %#v",
			result,
		)
	}
}

func dependencyForServer(
	t *testing.T,
	rawURL string,
	protocol catalog.Protocol,
	accepted []int,
) catalog.Dependency {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf(
			"url.Parse() error = %v",
			err,
		)
	}

	host, portString, err :=
		net.SplitHostPort(parsed.Host)

	if err != nil {
		t.Fatalf(
			"net.SplitHostPort() error = %v",
			err,
		)
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf(
			"strconv.Atoi() error = %v",
			err,
		)
	}

	return catalog.Dependency{
		Name:     "local",
		Protocol: protocol,
		Host:     host,
		Port:     uint16(port),
		HTTP: &catalog.HTTPCheck{
			Path: "/ready",

			AcceptedStatuses: accepted,
		},
	}
}

func assertReady(
	t *testing.T,
	result Result,
) {
	t.Helper()

	if result.Status != StatusReady ||
		result.Error != nil ||
		result.FailedStage != "" {
		t.Fatalf(
			"result = %#v, want ready",
			result,
		)
	}
}

func assertStage(
	t *testing.T,
	result Result,
	name StageName,
	want StageStatus,
) {
	t.Helper()

	for _, stage := range result.Stages {
		if stage.Name != name {
			continue
		}

		if stage.Status != want {
			t.Fatalf(
				"stage %s status = %s, want %s",
				name,
				stage.Status,
				want,
			)
		}

		return
	}

	t.Fatalf(
		"stage %s not found",
		name,
	)
}

func TestDialAttemptsErrorUnwraps(
	t *testing.T,
) {
	wrapped := &dialAttemptsError{
		errs: []error{
			syscall.ECONNREFUSED,
			context.DeadlineExceeded,
		},
	}

	if !errors.Is(
		wrapped,
		syscall.ECONNREFUSED,
	) {
		t.Fatal(
			"dialAttemptsError does not unwrap contained errors",
		)
	}
}
