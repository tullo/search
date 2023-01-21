package tracer

import (
	"log"

	"go.opentelemetry.io/otel/exporters/zipkin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init creates a new trace provider instance and registers it as global trace provider.
func Init(serviceName string, reporterURI string, probability float64, log *log.Logger) error {
	// Production mode configuarion
	// sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.ProbabilitySampler(probability)}),
	err := zipkin.InstallNewPipeline(reporterURI, serviceName,
		zipkin.WithSDK(&sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
	)
	if err != nil {
		return err
	}

	return nil
}
