package catalog

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maxDependencies      = 256
	maxDescriptionLength = 200
	maxHostnameLength    = 253
	maxHTTPPathLength    = 1024
	maxAcceptedStatuses  = 32
	maxTotalTimeout      = 30 * time.Second
	maxIndividualTimeout = 10 * time.Second
)

func buildCatalog(raw rawConfig) (*Catalog, error) {
	if raw.Version != CurrentVersion {
		return nil, invalidf("version must be %d", CurrentVersion)
	}

	timeouts, err := validateTimeouts(raw.Timeouts)
	if err != nil {
		return nil, err
	}

	if len(raw.Dependencies) == 0 {
		return nil, invalidf("dependencies must contain at least one entry")
	}
	if len(raw.Dependencies) > maxDependencies {
		return nil, invalidf("dependencies may contain at most %d entries", maxDependencies)
	}

	dependencies := make([]Dependency, 0, len(raw.Dependencies))
	seenNames := make(map[string]struct{}, len(raw.Dependencies))

	for i, rawDependency := range raw.Dependencies {
		dependency, err := validateDependency(i, rawDependency)
		if err != nil {
			return nil, err
		}

		if _, exists := seenNames[dependency.Name]; exists {
			return nil, invalidf(
				"dependencies[%d].name duplicates %q",
				i,
				dependency.Name,
			)
		}

		seenNames[dependency.Name] = struct{}{}
		dependencies = append(dependencies, dependency)
	}

	return newCatalog(timeouts, dependencies), nil
}

func validateTimeouts(raw rawTimeouts) (Timeouts, error) {
	fields := []struct {
		name  string
		value string
	}{
		{name: "timeouts.total", value: raw.Total},
		{name: "timeouts.dns", value: raw.DNS},
		{name: "timeouts.tcp", value: raw.TCP},
		{name: "timeouts.tls", value: raw.TLS},
		{name: "timeouts.http", value: raw.HTTP},
	}

	parsed := make([]time.Duration, len(fields))

	for i := range fields {
		duration, err := time.ParseDuration(fields[i].value)
		if err != nil {
			return Timeouts{}, invalidf(
				"%s must be a valid duration: %v",
				fields[i].name,
				err,
			)
		}

		if duration <= 0 {
			return Timeouts{}, invalidf(
				"%s must be greater than zero",
				fields[i].name,
			)
		}

		parsed[i] = duration
	}

	timeouts := Timeouts{
		Total: parsed[0],
		DNS:   parsed[1],
		TCP:   parsed[2],
		TLS:   parsed[3],
		HTTP:  parsed[4],
	}

	if timeouts.Total > maxTotalTimeout {
		return Timeouts{}, invalidf(
			"timeouts.total may not exceed %s",
			maxTotalTimeout,
		)
	}

	stages := []struct {
		name     string
		duration time.Duration
	}{
		{name: "timeouts.dns", duration: timeouts.DNS},
		{name: "timeouts.tcp", duration: timeouts.TCP},
		{name: "timeouts.tls", duration: timeouts.TLS},
		{name: "timeouts.http", duration: timeouts.HTTP},
	}

	for _, stage := range stages {
		if stage.duration > maxIndividualTimeout {
			return Timeouts{}, invalidf(
				"%s may not exceed %s",
				stage.name,
				maxIndividualTimeout,
			)
		}

		if stage.duration > timeouts.Total {
			return Timeouts{}, invalidf(
				"%s may not exceed timeouts.total",
				stage.name,
			)
		}
	}

	return timeouts, nil
}

func validateDependency(index int, raw rawDependency) (Dependency, error) {
	prefix := fmt.Sprintf("dependencies[%d]", index)

	if err := validateName(raw.Name); err != nil {
		return Dependency{}, invalidf("%s.name %v", prefix, err)
	}

	if raw.Description == "" ||
		strings.TrimSpace(raw.Description) != raw.Description {
		return Dependency{}, invalidf(
			"%s.description must be non-empty and have no leading or trailing whitespace",
			prefix,
		)
	}

	if len(raw.Description) > maxDescriptionLength {
		return Dependency{}, invalidf(
			"%s.description may not exceed %d bytes",
			prefix,
			maxDescriptionLength,
		)
	}

	if err := validateHost(raw.Host); err != nil {
		return Dependency{}, invalidf("%s.host %v", prefix, err)
	}

	if raw.Port < 1 || raw.Port > 65535 {
		return Dependency{}, invalidf(
			"%s.port must be between 1 and 65535",
			prefix,
		)
	}

	dependency := Dependency{
		Name:        raw.Name,
		Description: raw.Description,
		Protocol:    raw.Protocol,
		Host:        raw.Host,
		Port:        uint16(raw.Port),
	}

	switch raw.Protocol {
	case ProtocolTCP, ProtocolTLS:
		if raw.HTTP != nil {
			return Dependency{}, invalidf(
				"%s.http is not allowed when protocol is %q",
				prefix,
				raw.Protocol,
			)
		}

	case ProtocolHTTP, ProtocolHTTPS:
		if raw.HTTP == nil {
			return Dependency{}, invalidf(
				"%s.http is required when protocol is %q",
				prefix,
				raw.Protocol,
			)
		}

		httpCheck, err := validateHTTP(prefix+".http", *raw.HTTP)
		if err != nil {
			return Dependency{}, err
		}

		dependency.HTTP = &httpCheck

	default:
		return Dependency{}, invalidf(
			"%s.protocol must be one of %q, %q, %q, or %q",
			prefix,
			ProtocolTCP,
			ProtocolTLS,
			ProtocolHTTP,
			ProtocolHTTPS,
		)
	}

	return dependency, nil
}

func validateName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("must contain between 1 and 64 characters")
	}

	if name[0] < 'a' || name[0] > 'z' {
		return fmt.Errorf("must begin with a lowercase ASCII letter")
	}

	for _, char := range name[1:] {
		if (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '_' ||
			char == '-' {
			continue
		}

		return fmt.Errorf(
			"may contain only lowercase ASCII letters, digits, underscores, and hyphens",
		)
	}

	return nil
}

func validateHost(host string) error {
	if host == "" || strings.TrimSpace(host) != host {
		return fmt.Errorf(
			"must be non-empty and have no leading or trailing whitespace",
		)
	}

	if len(host) > maxHostnameLength {
		return fmt.Errorf("may not exceed %d bytes", maxHostnameLength)
	}

	if net.ParseIP(host) != nil {
		return nil
	}

	if strings.ContainsAny(host, "/\\:@?#[]") {
		return fmt.Errorf(
			"must be a hostname or IP address without a scheme, port, path, credentials, query, or fragment",
		)
	}

	if strings.HasSuffix(host, ".") {
		return fmt.Errorf("must not use a trailing dot")
	}

	for _, char := range host {
		if char > unicode.MaxASCII {
			return fmt.Errorf("must use ASCII hostname characters")
		}
	}

	labels := strings.Split(host, ".")

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("contains an empty or overlong DNS label")
		}

		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf(
				"DNS labels may not begin or end with a hyphen",
			)
		}

		for _, char := range label {
			if (char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') ||
				char == '-' {
				continue
			}

			return fmt.Errorf(
				"contains a DNS label with an unsupported character",
			)
		}
	}

	return nil
}

func validateHTTP(prefix string, raw rawHTTP) (HTTPCheck, error) {
	if raw.Path == "" || len(raw.Path) > maxHTTPPathLength {
		return HTTPCheck{}, invalidf(
			"%s.path must contain between 1 and %d bytes",
			prefix,
			maxHTTPPathLength,
		)
	}

	if !strings.HasPrefix(raw.Path, "/") ||
		strings.HasPrefix(raw.Path, "//") {
		return HTTPCheck{}, invalidf(
			"%s.path must begin with exactly one /",
			prefix,
		)
	}

	if strings.ContainsAny(raw.Path, "?#") {
		return HTTPCheck{}, invalidf(
			"%s.path must not contain a query string or fragment",
			prefix,
		)
	}

	for _, char := range raw.Path {
		if unicode.IsControl(char) {
			return HTTPCheck{}, invalidf(
				"%s.path must not contain control characters",
				prefix,
			)
		}
	}

	if len(raw.AcceptedStatuses) == 0 {
		return HTTPCheck{}, invalidf(
			"%s.accepted_statuses must contain at least one status code",
			prefix,
		)
	}

	if len(raw.AcceptedStatuses) > maxAcceptedStatuses {
		return HTTPCheck{}, invalidf(
			"%s.accepted_statuses may contain at most %d status codes",
			prefix,
			maxAcceptedStatuses,
		)
	}

	statuses := append([]int(nil), raw.AcceptedStatuses...)
	sort.Ints(statuses)

	for i, status := range statuses {
		if status < 100 || status > 599 {
			return HTTPCheck{}, invalidf(
				"%s.accepted_statuses contains invalid HTTP status %d",
				prefix,
				status,
			)
		}

		if i > 0 && statuses[i-1] == status {
			return HTTPCheck{}, invalidf(
				"%s.accepted_statuses contains duplicate HTTP status %d",
				prefix,
				status,
			)
		}
	}

	return HTTPCheck{
		Path:             raw.Path,
		AcceptedStatuses: statuses,
	}, nil
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf(
		"%w: %s",
		ErrInvalidConfiguration,
		fmt.Sprintf(format, args...),
	)
}
