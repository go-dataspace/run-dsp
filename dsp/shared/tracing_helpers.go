package shared

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type TraceInfo struct {
	TraceID    string
	SpanID     string
	TraceFlags byte
}

func ExtractTraceInfo(ctx context.Context) TraceInfo {
	spanContext := trace.SpanFromContext(ctx).SpanContext()
	return TraceInfo{
		TraceID:    spanContext.TraceID().String(),
		SpanID:     spanContext.SpanID().String(),
		TraceFlags: byte(spanContext.TraceFlags()),
	}
}

func RestoreSpanContext(info TraceInfo, remote bool) trace.SpanContext {
	traceId, _ := trace.TraceIDFromHex(info.TraceID)
	spanId, _ := trace.SpanIDFromHex(info.SpanID)
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceId,
		SpanID:     spanId,
		TraceFlags: trace.TraceFlags(info.TraceFlags),
		Remote:     remote,
	})
}
