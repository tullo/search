package tracer

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

// Init creates a new trace provider instance and registers it as global trace provider.
func Init(serviceName string, reporterURI string, probability float64, log *log.Logger) (func(context.Context) error, error) {
	exporter, err := zipkin.New(reporterURI, zipkin.WithLogger(log))
	if err != nil {
		return nil, err
	}

	batcher := sdktrace.NewBatchSpanProcessor(exporter)

	// By default the returned TracerProvider is configured with:
	// - a ParentBased(AlwaysSample) Sampler
	// - a random number IDGenerator
	// - the resource.Default() Resource
	// - the default SpanLimits
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("zipkin-search"),
		)),
	)
	// TODO: Production mode configuarion
	// TODO: sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.ProbabilitySampler(probability)}),

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
