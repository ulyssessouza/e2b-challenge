package service

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("e2b-challenge")