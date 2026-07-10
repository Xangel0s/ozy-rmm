package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
)

type ctxKey string

const RequestIDKey ctxKey = "request_id"

func NewRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func NewContextWithRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, RequestIDKey, NewRequestID())
}

func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

func SlogWithRequestID(ctx context.Context, msg string, attrs ...any) {
	id := GetRequestID(ctx)
	if id != "" {
		attrs = append([]any{slog.String("request_id", id)}, attrs...)
	}
	slog.Info(msg, attrs...)
}

func SlogErrorWithRequestID(ctx context.Context, msg string, attrs ...any) {
	id := GetRequestID(ctx)
	if id != "" {
		attrs = append([]any{slog.String("request_id", id)}, attrs...)
	}
	slog.Error(msg, attrs...)
}
