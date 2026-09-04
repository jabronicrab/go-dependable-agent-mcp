package preflight

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
)

const maxResponseHeaderBytes = 64 << 10

const (
	dnsStageIndex = iota
	tcpStageIndex
	tlsStageIndex
	httpStageIndex
)

type resolver interface {
	LookupHost(
		context.Context,
		string,
	) ([]string, error)
}

type dialer interface {
	DialContext(
		context.Context,
		string,
		string,
	) (net.Conn, error)
}

// Checker performs bounded DNS, TCP, optional TLS, and optional HTTP checks.
type Checker struct {
	resolver resolver
	dialer   dialer
	rootCAs  *x509.CertPool
	now      func() time.Time
}

// NewChecker constructs the production readiness checker using the operating
// system resolver, network stack, and certificate trust store.
func NewChecker() *Checker {
	return &Checker{
		resolver: net.DefaultResolver,
		dialer:   &net.Dialer{},
		now:      time.Now,
	}
}

// Check executes the applicable readiness stages sequentially under a total
// deadline and per-stage deadline ceilings.
func (c *Checker) Check(
	ctx context.Context,
	dependency catalog.Dependency,
	timeouts catalog.Timeouts,
) Result {
	startedAt := c.now()
	stages := initialStages(dependency.Protocol)

	if !knownProtocol(dependency.Protocol) {
		return c.internalFailure(
			dependency,
			startedAt,
			stages,
		)
	}

	totalCtx, cancelTotal := context.WithTimeout(
		ctx,
		timeouts.Total,
	)
	defer cancelTotal()

	addresses := []string{dependency.Host}

	if net.ParseIP(dependency.Host) == nil {
		stageStartedAt := c.now()

		stageCtx, cancelStage := context.WithTimeout(
			totalCtx,
			timeouts.DNS,
		)

		resolved, err := c.resolver.LookupHost(
			stageCtx,
			dependency.Host,
		)

		if err == nil && len(resolved) == 0 {
			err = errNoResolvedAddresses
		}

		stages[dnsStageIndex].DurationMS =
			elapsedMilliseconds(
				stageStartedAt,
				c.now(),
			)

		if err != nil {
			failure := classifyDNS(stageCtx, err)
			cancelStage()

			stages[dnsStageIndex].Status =
				StageFailed

			return c.failureResult(
				dependency,
				startedAt,
				stages,
				StageDNS,
				failure,
			)
		}

		cancelStage()

		addresses = resolved
		stages[dnsStageIndex].Status =
			StagePassed
	} else {
		stages[dnsStageIndex].Status =
			StageNotApplicable
	}

	stageStartedAt := c.now()

	tcpCtx, cancelTCP := context.WithTimeout(
		totalCtx,
		timeouts.TCP,
	)

	conn, err := c.dialResolved(
		tcpCtx,
		addresses,
		dependency.Port,
	)

	stages[tcpStageIndex].DurationMS =
		elapsedMilliseconds(
			stageStartedAt,
			c.now(),
		)

	if err != nil {
		failure := classifyTCP(tcpCtx, err)
		cancelTCP()

		stages[tcpStageIndex].Status =
			StageFailed

		return c.failureResult(
			dependency,
			startedAt,
			stages,
			StageTCP,
			failure,
		)
	}

	cancelTCP()

	stages[tcpStageIndex].Status =
		StagePassed

	activeConn := conn

	defer func() {
		_ = activeConn.Close()
	}()

	if dependency.Protocol == catalog.ProtocolTLS ||
		dependency.Protocol == catalog.ProtocolHTTPS {
		stageStartedAt = c.now()

		tlsCtx, cancelTLS := context.WithTimeout(
			totalCtx,
			timeouts.TLS,
		)

		tlsConn := tls.Client(
			activeConn,
			&tls.Config{
				ServerName: dependency.Host,
				RootCAs:    c.rootCAs,
				MinVersion: tls.VersionTLS12,
				NextProtos: []string{
					"http/1.1",
				},
			},
		)

		activeConn = tlsConn

		err = tlsConn.HandshakeContext(tlsCtx)

		stages[tlsStageIndex].DurationMS =
			elapsedMilliseconds(
				stageStartedAt,
				c.now(),
			)

		if err != nil {
			failure := classifyTLS(
				tlsCtx,
				err,
			)

			cancelTLS()

			stages[tlsStageIndex].Status =
				StageFailed

			return c.failureResult(
				dependency,
				startedAt,
				stages,
				StageTLS,
				failure,
			)
		}

		cancelTLS()

		stages[tlsStageIndex].Status =
			StagePassed
	}

	if dependency.Protocol == catalog.ProtocolHTTP ||
		dependency.Protocol == catalog.ProtocolHTTPS {
		stageStartedAt = c.now()

		httpCtx, cancelHTTP := context.WithTimeout(
			totalCtx,
			timeouts.HTTP,
		)

		_, err = c.checkHTTP(
			httpCtx,
			activeConn,
			dependency,
		)

		stages[httpStageIndex].DurationMS =
			elapsedMilliseconds(
				stageStartedAt,
				c.now(),
			)

		if err != nil {
			failure := classifyHTTP(
				httpCtx,
				err,
			)

			cancelHTTP()

			stages[httpStageIndex].Status =
				StageFailed

			return c.failureResult(
				dependency,
				startedAt,
				stages,
				StageHTTP,
				failure,
			)
		}

		cancelHTTP()

		stages[httpStageIndex].Status =
			StagePassed
	}

	return Result{
		Dependency: dependency.Name,
		Status:     StatusReady,
		CheckedAt:  startedAt.UTC(),
		DurationMS: elapsedMilliseconds(
			startedAt,
			c.now(),
		),
		Stages: stages,
	}
}

func (c *Checker) dialResolved(
	ctx context.Context,
	addresses []string,
	port uint16,
) (net.Conn, error) {
	if len(addresses) == 0 {
		return nil, errNoResolvedAddresses
	}

	attempts := make(
		[]error,
		0,
		len(addresses),
	)

	for _, address := range addresses {
		conn, err := c.dialer.DialContext(
			ctx,
			"tcp",
			net.JoinHostPort(
				address,
				strconv.Itoa(int(port)),
			),
		)

		if err == nil {
			return conn, nil
		}

		attempts = append(
			attempts,
			fmt.Errorf(
				"dial resolved address: %w",
				err,
			),
		)

		if ctx.Err() != nil {
			break
		}
	}

	return nil, &dialAttemptsError{
		errs: attempts,
	}
}

func (c *Checker) checkHTTP(
	ctx context.Context,
	conn net.Conn,
	dependency catalog.Dependency,
) (int, error) {
	if dependency.HTTP == nil {
		return 0, errors.New(
			"HTTP configuration is missing",
		)
	}

	scheme := "http"

	if dependency.Protocol == catalog.ProtocolHTTPS {
		scheme = "https"
	}

	target := &url.URL{
		Scheme: scheme,
		Host: net.JoinHostPort(
			dependency.Host,
			strconv.Itoa(int(dependency.Port)),
		),
		Path: dependency.HTTP.Path,
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		target.String(),
		nil,
	)

	if err != nil {
		return 0, fmt.Errorf(
			"construct HTTP readiness request: %w",
			err,
		)
	}

	provider := &singleConnProvider{
		conn: conn,
	}

	transport := &http.Transport{
		Proxy:              nil,
		DialContext:        provider.dialContext,
		DialTLSContext:     provider.dialContext,
		DisableKeepAlives:  true,
		DisableCompression: true,
		ForceAttemptHTTP2:  false,

		MaxResponseHeaderBytes: maxResponseHeaderBytes,

		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}

	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,

		CheckRedirect: func(
			*http.Request,
			[]*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	}

	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf(
			"perform HTTP readiness request: %w",
			err,
		)
	}

	defer response.Body.Close()

	if !acceptedStatus(
		response.StatusCode,
		dependency.HTTP.AcceptedStatuses,
	) {
		return response.StatusCode,
			&unexpectedHTTPStatusError{
				status: response.StatusCode,

				accepted: append(
					[]int(nil),
					dependency.HTTP.
						AcceptedStatuses...,
				),
			}
	}

	return response.StatusCode, nil
}

func initialStages(
	protocol catalog.Protocol,
) []StageResult {
	stages := []StageResult{
		{
			Name:   StageDNS,
			Status: StageNotAttempted,
		},
		{
			Name:   StageTCP,
			Status: StageNotAttempted,
		},
		{
			Name:   StageTLS,
			Status: StageNotAttempted,
		},
		{
			Name:   StageHTTP,
			Status: StageNotAttempted,
		},
	}

	if protocol == catalog.ProtocolTCP ||
		protocol == catalog.ProtocolHTTP {
		stages[tlsStageIndex].Status =
			StageNotApplicable
	}

	if protocol == catalog.ProtocolTCP ||
		protocol == catalog.ProtocolTLS {
		stages[httpStageIndex].Status =
			StageNotApplicable
	}

	return stages
}

func knownProtocol(
	protocol catalog.Protocol,
) bool {
	switch protocol {
	case catalog.ProtocolTCP,
		catalog.ProtocolTLS,
		catalog.ProtocolHTTP,
		catalog.ProtocolHTTPS:
		return true

	default:
		return false
	}
}

func acceptedStatus(
	status int,
	accepted []int,
) bool {
	for _, candidate := range accepted {
		if status == candidate {
			return true
		}
	}

	return false
}

func elapsedMilliseconds(
	start time.Time,
	end time.Time,
) int64 {
	duration := end.Sub(start)

	if duration < 0 {
		return 0
	}

	return duration.Milliseconds()
}

func (c *Checker) failureResult(
	dependency catalog.Dependency,
	startedAt time.Time,
	stages []StageResult,
	failedStage StageName,
	failure *Failure,
) Result {
	return Result{
		Dependency: dependency.Name,
		Status:     StatusNotReady,
		CheckedAt:  startedAt.UTC(),
		DurationMS: elapsedMilliseconds(
			startedAt,
			c.now(),
		),
		FailedStage: failedStage,
		Error:       failure,
		Stages:      stages,
	}
}

func (c *Checker) internalFailure(
	dependency catalog.Dependency,
	startedAt time.Time,
	stages []StageResult,
) Result {
	return Result{
		Dependency: dependency.Name,
		Status:     StatusNotReady,
		CheckedAt:  startedAt.UTC(),
		DurationMS: elapsedMilliseconds(
			startedAt,
			c.now(),
		),
		Error: &Failure{
			Category: ErrorInternal,
			Message: "dependency configuration is " +
				"inconsistent with the readiness checker",
		},
		Stages: stages,
	}
}

type dialAttemptsError struct {
	errs []error
}

func (e *dialAttemptsError) Error() string {
	return "all TCP connection attempts failed"
}

func (e *dialAttemptsError) Unwrap() []error {
	return e.errs
}

type singleConnProvider struct {
	mu   sync.Mutex
	conn net.Conn
	used bool
}

func (p *singleConnProvider) dialContext(
	ctx context.Context,
	_ string,
	_ string,
) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.used {
		return nil, errors.New(
			"preconnected readiness connection " +
				"has already been consumed",
		)
	}

	p.used = true

	return p.conn, nil
}
