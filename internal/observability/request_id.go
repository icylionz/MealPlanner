package observability

import "context"

const RequestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

// SafeRequestID reports whether an externally supplied ID is safe for response
// headers and structured logs.
func SafeRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := range len(id) {
		b := id[i]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			continue
		}
		switch b {
		case '-', '_', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}
