package preflight

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResultJSONContract(t *testing.T) {
	result := Result{
		Dependency: "demo_unhealthy",
		Status:     StatusNotReady,
		CheckedAt: time.Date(
			2026,
			time.September,
			3,
			23,
			15,
			27,
			123000000,
			time.UTC,
		),
		DurationMS:  2,
		FailedStage: StageHTTP,
		Error: &Failure{
			Category:   ErrorHTTPUnexpectedStatus,
			Message:    "received HTTP status 503; accepted statuses are [200]",
			HTTPStatus: 503,
		},
		Stages: []StageResult{
			{
				Name:       StageDNS,
				Status:     StagePassed,
				DurationMS: 0,
			},
			{
				Name:       StageTCP,
				Status:     StagePassed,
				DurationMS: 1,
			},
			{
				Name:       StageTLS,
				Status:     StageNotApplicable,
				DurationMS: 0,
			},
			{
				Name:       StageHTTP,
				Status:     StageFailed,
				DurationMS: 1,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := `{"dependency":"demo_unhealthy","status":"not_ready","checked_at":"2026-09-03T23:15:27.123Z","duration_ms":2,"failed_stage":"http","error":{"category":"http_unexpected_status","message":"received HTTP status 503; accepted statuses are [200]","http_status":503},"stages":[{"name":"dns","status":"passed","duration_ms":0},{"name":"tcp","status":"passed","duration_ms":1},{"name":"tls","status":"not_applicable","duration_ms":0},{"name":"http","status":"failed","duration_ms":1}]}`

	if got := string(data); got != want {
		t.Fatalf(
			"JSON = %s\nwant = %s",
			got,
			want,
		)
	}
}

func TestReadyResultOmitsFailureFields(t *testing.T) {
	result := Result{
		Dependency: "demo_ready",
		Status:     StatusReady,
		CheckedAt: time.Date(
			2026,
			time.September,
			3,
			23,
			15,
			22,
			0,
			time.UTC,
		),
		DurationMS: 4,
		Stages: []StageResult{
			{
				Name:   StageDNS,
				Status: StagePassed,
			},
			{
				Name:       StageTCP,
				Status:     StagePassed,
				DurationMS: 1,
			},
			{
				Name:   StageTLS,
				Status: StageNotApplicable,
			},
			{
				Name:       StageHTTP,
				Status:     StagePassed,
				DurationMS: 3,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, exists := decoded["failed_stage"]; exists {
		t.Fatal("ready result contains failed_stage")
	}

	if _, exists := decoded["error"]; exists {
		t.Fatal("ready result contains error")
	}
}
