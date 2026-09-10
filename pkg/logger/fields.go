package logger

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/perfect-panel/server/pkg/requestmeta"
)

var (
	fieldsContextKey contextKey
	globalFields     atomic.Value
	globalFieldsLock sync.Mutex
)

// ContextWithRequestMetadata adds explicitly authorized risk metadata to all
// process logs emitted with this context. Credentials, bodies and other
// personal fields remain governed by the normal redaction policy.
func ContextWithRequestMetadata(ctx context.Context, metadata requestmeta.Metadata) context.Context {
	metadata = requestmeta.Normalize(metadata)
	fields := make([]LogField, 0, 9)
	if metadata.ClientIP != "" {
		fields = append(fields, RiskField("client_ip", metadata.ClientIP))
	}
	if metadata.UserAgent != "" {
		fields = append(fields, RiskField("user_agent", metadata.UserAgent))
	}
	if metadata.ActorID > 0 {
		fields = append(fields, Field("actor_id", metadata.ActorID))
	}
	if metadata.IPCountryCode != "" {
		fields = append(fields, Field("ip_country_code", metadata.IPCountryCode))
	}
	if metadata.IPCountry != "" {
		fields = append(fields, Field("ip_country", metadata.IPCountry))
	}
	if metadata.IPRegion != "" {
		fields = append(fields, Field("ip_region", metadata.IPRegion))
	}
	if metadata.IPCity != "" {
		fields = append(fields, Field("ip_city", metadata.IPCity))
	}
	if metadata.IPASN > 0 {
		fields = append(fields, Field("ip_asn", metadata.IPASN))
	}
	if metadata.IPASOrganization != "" {
		fields = append(fields, Field("ip_as_organization", metadata.IPASOrganization))
	}
	if len(fields) == 0 {
		return ctx
	}
	return ContextWithFields(ctx, fields...)
}

type contextKey struct{}

// AddGlobalFields adds global fields.
func AddGlobalFields(fields ...LogField) {
	globalFieldsLock.Lock()
	defer globalFieldsLock.Unlock()

	old := globalFields.Load()
	if old == nil {
		globalFields.Store(append([]LogField(nil), fields...))
	} else {
		globalFields.Store(append(old.([]LogField), fields...))
	}
}

// ContextWithFields returns a new context with the given fields.
func ContextWithFields(ctx context.Context, fields ...LogField) context.Context {
	if val := ctx.Value(fieldsContextKey); val != nil {
		if arr, ok := val.([]LogField); ok {
			allFields := make([]LogField, 0, len(arr)+len(fields))
			allFields = append(allFields, arr...)
			allFields = append(allFields, fields...)
			return context.WithValue(ctx, fieldsContextKey, allFields)
		}
	}

	return context.WithValue(ctx, fieldsContextKey, fields)
}

// WithFields returns a new logger with the given fields.
// deprecated: use ContextWithFields instead.
func WithFields(ctx context.Context, fields ...LogField) context.Context {
	return ContextWithFields(ctx, fields...)
}
