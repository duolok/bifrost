package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init sets up the OpenTelemetry tracer with an OTLP gRPC exporter.
// Returns a shutdown function that should be deferred.
func Init(ctx context.Context, serviceName string) func() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		slog.Info("OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled")
		return func() {}
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		slog.Error("failed to create OTLP exporter", "error", err)
		return func() {}
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracing enabled", "endpoint", endpoint, "service", serviceName)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown tracer", "error", err)
		}
	}
}
