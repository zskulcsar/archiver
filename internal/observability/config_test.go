package observability

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// * [x] **OTEL_001** Explicit endpoint overrides environment configuration
// - Description: Resolves telemetry configuration with both an explicit CLI endpoint and an environment endpoint.
// - Expected: The explicit endpoint is selected without changing the process environment.
func TestResolveEndpoint_PrefersExplicitEndpoint(t *testing.T) {
	t.Parallel()

	got := ResolveEndpoint("http://127.0.0.1:4318", "http://collector.example:4318")
	if want := "http://127.0.0.1:4318"; got != want {
		t.Fatalf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

// * [x] **OTEL_002** Empty configuration leaves telemetry disabled
// - Description: Resolves telemetry configuration without CLI or environment endpoints.
// - Expected: No endpoint is returned so archive operations use OpenTelemetry no-op providers.
func TestResolveEndpoint_LeavesTelemetryDisabled(t *testing.T) {
	t.Parallel()

	if got := ResolveEndpoint("", ""); got != "" {
		t.Fatalf("ResolveEndpoint() = %q, want empty endpoint", got)
	}
}

// * [x] **OTEL_003** Signal endpoints use OTLP HTTP paths
// - Description: Builds a trace-export URL from an explicit OTLP base endpoint.
// - Expected: The trace signal path is appended without retaining query or fragment data.
func TestSignalURL_AppendsSignalPath(t *testing.T) {
	t.Parallel()

	got, err := signalURL("http://127.0.0.1:4318?ignored=true#ignored", "v1/traces")
	if err != nil {
		t.Fatalf("signalURL() error = %v", err)
	}
	if want := "http://127.0.0.1:4318/v1/traces"; got != want {
		t.Fatalf("signalURL() = %q, want %q", got, want)
	}
}

// * [x] **OTEL_004** Telemetry errors exclude unsafe error messages
// - Description: Records a failure whose message contains a private source path on an in-memory span.
// - Expected: The span is marked as failed without exception events or the private message.
func TestRecordError_ExcludesErrorMessage(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	_, span := provider.Tracer("test").Start(t.Context(), "operation")
	RecordError(span, errors.New("read /private/source/file: permission denied"))
	span.End()

	spans := exporter.GetSpans()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("exported spans = %d, want %d", got, want)
	}
	if got, want := spans[0].Status.Code, codes.Error; got != want {
		t.Fatalf("span status = %v, want %v", got, want)
	}
	if got := spans[0].Events; len(got) != 0 {
		t.Fatalf("span events = %#v, want no raw error events", got)
	}
	for _, attribute := range spans[0].Attributes {
		if attribute.Value.AsString() == "read /private/source/file: permission denied" {
			t.Fatalf("span attributes expose error message: %#v", spans[0].Attributes)
		}
	}
}
