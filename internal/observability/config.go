// Package observability provides optional OpenTelemetry instrumentation for Archiver.
package observability

// ResolveEndpoint selects an explicit CLI endpoint over environment configuration.
func ResolveEndpoint(explicit, environment string) string {
	if explicit != "" {
		return explicit
	}
	return environment
}
