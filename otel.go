package volt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// OTELProvider manages OpenTelemetry providers and exporters.
type OTELProvider struct {
	config         OTELConfig
	resource       *resource.Resource
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	logger         *slog.Logger
}

// NewOTELProvider creates and configures OpenTelemetry providers.
func NewOTELProvider(config OTELConfig) (*OTELProvider, error) {
	ctx := context.Background()

	// Build resource with service information
	res, err := buildResource(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := &OTELProvider{
		config:   config,
		resource: res,
	}

	// Setup trace provider
	if config.EnableTraces {
		tp, err := provider.setupTraceProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create trace provider: %w", err)
		}
		provider.tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	// Setup meter provider
	if config.EnableMetrics {
		mp, err := provider.setupMeterProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create meter provider: %w", err)
		}
		provider.meterProvider = mp
		otel.SetMeterProvider(mp)
	}

	// Setup log provider
	if config.EnableLogs {
		lp, err := provider.setupLogProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create log provider: %w", err)
		}
		provider.loggerProvider = lp
		global.SetLoggerProvider(lp)

		// Create slog handler bridged to OTEL
		provider.logger = slog.New(otelslog.NewHandler(config.ServiceName))
	}

	// Setup propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider, nil
}

// buildResource creates the OTEL resource with service attributes.
func buildResource(config OTELConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.DeploymentEnvironment(config.Environment),
	}

	// Add custom attributes
	for k, v := range config.Attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

// setupTraceProvider creates the trace provider with OTLP exporter.
func (p *OTELProvider) setupTraceProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(p.config.CollectorURL),
		otlptracegrpc.WithInsecure(), // Use WithTLSCredentials in production
	)
	if err != nil {
		return nil, err
	}

	// Configure sampler
	var sampler sdktrace.Sampler
	if p.config.TraceSampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if p.config.TraceSampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(p.config.TraceSampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(p.resource),
		sdktrace.WithSampler(sampler),
	)

	return tp, nil
}

// setupMeterProvider creates the meter provider with OTLP exporter.
func (p *OTELProvider) setupMeterProvider(ctx context.Context) (*sdkmetric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(p.config.CollectorURL),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(30*time.Second),
			),
		),
		sdkmetric.WithResource(p.resource),
	)

	return mp, nil
}

// setupLogProvider creates the log provider with OTLP exporter.
func (p *OTELProvider) setupLogProvider(ctx context.Context) (*sdklog.LoggerProvider, error) {
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(p.config.CollectorURL),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(
			sdklog.NewBatchProcessor(exporter),
		),
		sdklog.WithResource(p.resource),
	)

	return lp, nil
}

// TracerProvider returns the trace provider.
func (p *OTELProvider) TracerProvider() trace.TracerProvider {
	if p.tracerProvider != nil {
		return p.tracerProvider
	}
	return otel.GetTracerProvider()
}

// MeterProvider returns the meter provider.
func (p *OTELProvider) MeterProvider() metric.MeterProvider {
	if p.meterProvider != nil {
		return p.meterProvider
	}
	return otel.GetMeterProvider()
}

// Logger returns the OTEL-bridged slog logger.
func (p *OTELProvider) Logger() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return slog.Default()
}

// Tracer returns a named tracer.
func (p *OTELProvider) Tracer(name string) trace.Tracer {
	return p.TracerProvider().Tracer(name)
}

// Meter returns a named meter.
func (p *OTELProvider) Meter(name string) metric.Meter {
	return p.MeterProvider().Meter(name)
}

// Shutdown gracefully shuts down all providers.
func (p *OTELProvider) Shutdown(ctx context.Context) error {
	var errs []error

	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace provider shutdown: %w", err))
		}
	}

	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
		}
	}

	if p.loggerProvider != nil {
		if err := p.loggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("log provider shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("otel shutdown errors: %v", errs)
	}
	return nil
}

// --- Convenience Functions for Application Code ---

// StartSpan starts a new span in the given context.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("volt").Start(ctx, name, opts...)
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
	}
}

// SetSpanAttributes sets attributes on the current span.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// --- Metrics Helpers ---

// Counter creates a new counter metric.
func Counter(name, description string) (metric.Int64Counter, error) {
	return otel.Meter("volt").Int64Counter(name,
		metric.WithDescription(description),
	)
}

// Histogram creates a new histogram metric.
func Histogram(name, description string, buckets ...float64) (metric.Float64Histogram, error) {
	return otel.Meter("volt").Float64Histogram(name,
		metric.WithDescription(description),
	)
}

// Gauge creates a new gauge metric via callback.
func Gauge(name, description string, callback func() int64) (metric.Registration, error) {
	gauge, err := otel.Meter("volt").Int64ObservableGauge(name,
		metric.WithDescription(description),
	)
	if err != nil {
		return nil, err
	}

	return otel.Meter("volt").RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			o.ObserveInt64(gauge, callback())
			return nil
		},
		gauge,
	)
}
