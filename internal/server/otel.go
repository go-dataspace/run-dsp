package server

import (
	"context"
	"errors"

	"go-dataspace.eu/ctxslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

type NullExporter struct{}

func (e *NullExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	return nil
}

func (e *NullExporter) Start(ctx context.Context) error {
	return nil
}

func (e *NullExporter) Shutdown(ctx context.Context) error {
	return nil
}

func setupOTelSDK(
	ctx context.Context, enabled bool, endpointUrl string, serviceName string,
) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, enabled, endpointUrl, serviceName)
	if err != nil {
		err = errors.Join(err, shutdown(ctx))
		return nil, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)
	return shutdown, nil
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(
	ctx context.Context, enabled bool, endpointUrl string, serviceName string,
) (*trace.TracerProvider, error) {
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	if !enabled {
		ctxslog.Info(ctx, "Setting null exporter for opentelemetry data")
		return trace.NewTracerProvider(
			trace.WithBatcher(&NullExporter{}),
		), nil
	}

	ctxslog.Info(ctx, "Setting up opentelemetry HTTP exporter",
		"endpoint_url", endpointUrl, "service_name", serviceName)
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpointUrl))
	if err != nil {
		return nil, err
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)
	return tracerProvider, nil
}
