package observability

import (
	"context"
	"fmt"
	"net/http"

	"github.com/configkits/mcp-gateway/internal/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const serviceName = "mcp-gateway"

// InitTracer sets up the OpenTelemetry trace provider.
// Returns a shutdown function that flushes pending spans.
func InitTracer(endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		// No-op tracer
		return func(ctx context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, _ := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version.String),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// metricsData holds simple in-process counters.
var metricsData = struct {
	requestsTotal    int64
	requestsSuccess  int64
	requestsError    int64
	backendTimeoutMs int64
}{}

// MetricsHandler returns a simple text/plain Prometheus-compatible metrics endpoint.
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP mcp_gateway_requests_total Total requests proxied\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_requests_total counter\n")
		fmt.Fprintf(w, "mcp_gateway_requests_total %d\n", metricsData.requestsTotal)
		fmt.Fprintf(w, "# HELP mcp_gateway_requests_success Successful proxied requests\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_requests_success counter\n")
		fmt.Fprintf(w, "mcp_gateway_requests_success %d\n", metricsData.requestsSuccess)
		fmt.Fprintf(w, "# HELP mcp_gateway_requests_error Failed proxied requests\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_requests_error counter\n")
		fmt.Fprintf(w, "mcp_gateway_requests_error %d\n", metricsData.requestsError)
	})
}
