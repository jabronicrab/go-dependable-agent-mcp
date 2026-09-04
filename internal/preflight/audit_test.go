package preflight

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
)

func TestCheckerExplainsSkippedStagesAfterFailure(t *testing.T) {
	resolver := &fakeResolver{
		err: &contextError{err: context.DeadlineExceeded},
	}

	checker := &Checker{
		resolver: resolver,
		dialer:   &fakeDialer{},
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolHTTPS,
		Host:     "api.example",
		Port:     443,
		HTTP: &catalog.HTTPCheck{
			Path:             "/ready",
			AcceptedStatuses: []int{http.StatusOK},
		},
	}

	result := checker.Check(context.Background(), dependency, testTimeouts)

	assertStageReason(t, result, StageTCP, StageReasonPriorStageFailed)
	assertStageReason(t, result, StageTLS, StageReasonPriorStageFailed)
	assertStageReason(t, result, StageHTTP, StageReasonPriorStageFailed)
}

func TestCheckerExplainsNotApplicableStages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolHTTP,
		[]int{http.StatusNoContent},
	)

	result := NewChecker().Check(context.Background(), dependency, testTimeouts)

	assertStageReason(t, result, StageDNS, StageReasonLiteralIP)
	assertStageReason(t, result, StageTLS, StageReasonProtocolNotApplicable)
}

func TestCheckerTLSProtocolDoesNotAdvertiseHTTPALPN(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			if len(hello.SupportedProtos) != 0 {
				return nil, errors.New("TLS-only readiness check advertised an application protocol")
			}
			return nil, nil
		},
	}
	server.StartTLS()
	defer server.Close()

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())

	checker := NewChecker()
	checker.rootCAs = pool

	dependency := dependencyForServer(
		t,
		server.URL,
		catalog.ProtocolTLS,
		nil,
	)
	dependency.HTTP = nil

	result := checker.Check(context.Background(), dependency, testTimeouts)

	assertReady(t, result)
	assertStage(t, result, StageTLS, StagePassed)
	assertStageReason(t, result, StageHTTP, StageReasonProtocolNotApplicable)
}

func TestCheckerPreservesDiagnosticCauseWithoutJSONLeak(t *testing.T) {
	diagnostic := errors.New("private diagnostic sentinel")

	checker := &Checker{
		resolver: &fakeResolver{addresses: []string{"127.0.0.1"}},
		dialer:   &fakeDialer{err: diagnostic},
		now:      time.Now,
	}

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolTCP,
		Host:     "api.example",
		Port:     443,
	}

	result := checker.Check(context.Background(), dependency, testTimeouts)

	if !errors.Is(result.Cause(), diagnostic) {
		t.Fatalf("Cause() = %v, want wrapped diagnostic sentinel", result.Cause())
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if strings.Contains(string(data), diagnostic.Error()) {
		t.Fatalf("serialized result leaked diagnostic cause: %s", data)
	}
}

func TestCheckerStageDeadlineCancelsResolver(t *testing.T) {
	checker := &Checker{
		resolver: waitForContextResolver{},
		dialer:   &fakeDialer{},
		now:      time.Now,
	}

	timeouts := testTimeouts
	timeouts.Total = 200 * time.Millisecond
	timeouts.DNS = 20 * time.Millisecond

	dependency := catalog.Dependency{
		Name:     "api",
		Protocol: catalog.ProtocolTCP,
		Host:     "api.example",
		Port:     443,
	}

	result := checker.Check(context.Background(), dependency, timeouts)

	if result.FailedStage != StageDNS ||
		result.Error == nil ||
		result.Error.Category != ErrorDNSTimeout {
		t.Fatalf("result = %#v", result)
	}
}

type waitForContextResolver struct{}

func (waitForContextResolver) LookupHost(ctx context.Context, _ string) ([]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type contextError struct {
	err error
}

func (e *contextError) Error() string {
	return e.err.Error()
}

func (e *contextError) Unwrap() error {
	return e.err
}

func assertStageReason(
	t *testing.T,
	result Result,
	name StageName,
	want StageReason,
) {
	t.Helper()

	for _, stage := range result.Stages {
		if stage.Name != name {
			continue
		}

		if stage.Reason != want {
			t.Fatalf(
				"stage %s reason = %q, want %q; result = %#v",
				name,
				stage.Reason,
				want,
				result,
			)
		}
		return
	}

	t.Fatalf("stage %s not found", name)
}
