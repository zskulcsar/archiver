package observability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/zskulcsar/archiver"

// Config configures optional OTLP/HTTP telemetry export.
type Config struct {
	Endpoint string
	Version  string
	Revision string
}

// Initialize configures OpenTelemetry when endpoint is non-empty.
// An empty endpoint leaves the global no-op providers in place.
func Initialize(ctx context.Context, config Config) (func(context.Context) error, error) {
	if config.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	traceURL, err := signalURL(config.Endpoint, "v1/traces")
	if err != nil {
		return nil, err
	}
	metricURL, err := signalURL(config.Endpoint, "v1/metrics")
	if err != nil {
		return nil, err
	}
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(traceURL))
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(metricURL))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create OTLP metric exporter: %w", err), traceExporter.Shutdown(ctx))
	}
	attributes := []attribute.KeyValue{
		attribute.String("service.name", "archiver"),
		attribute.String("service.version", config.Version),
	}
	if config.Revision != "" {
		attributes = append(attributes, attribute.String("vcs.revision", config.Revision))
	}
	res := resource.NewWithAttributes("", attributes...)
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)), sdkmetric.WithResource(res))
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	return func(ctx context.Context) error {
		return errors.Join(meterProvider.Shutdown(ctx), tracerProvider.Shutdown(ctx))
	}, nil
}

// Start starts an Archiver operation span with privacy-safe attributes.
func Start(ctx context.Context, name string, attributes ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(instrumentationName).Start(ctx, name, trace.WithAttributes(attributes...))
}

// RecordError marks an operation failure without recording an unsafe error message or exception event.
func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.SetAttributes(attribute.String("archiver.error.category", "operation_failed"))
	span.SetStatus(codes.Error, "operation failed")
}

// RecordIO records bytes read or written by a named low-cardinality operation.
func RecordIO(ctx context.Context, operation string, bytes int64) {
	if bytes <= 0 {
		return
	}
	counter, err := otel.Meter(instrumentationName).Int64Counter("archiver.io.bytes", otelmetric.WithUnit("By"))
	if err != nil {
		return
	}
	counter.Add(ctx, bytes, otelmetric.WithAttributes(attribute.String("archiver.io.operation", operation)))
}

func signalURL(endpoint, signal string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid OTLP endpoint %q", endpoint)
	}
	parsed.Path = "/" + strings.TrimPrefix(signal, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// ShutdownTimeout bounds exporter flushing during CLI shutdown.
const ShutdownTimeout = 5 * time.Second
