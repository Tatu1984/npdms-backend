// Package audit carries what is known about a request to whoever writes the
// audit entry for it.
//
// The immutable schema from migration 000016 has columns for the client
// address, the session, the device, the user agent and the caller's place in
// the force. Twenty-five of its forty-one columns had never been written to.
// Not because the information was unavailable — the middleware already knows
// all of it — but because the only way to record it was for each of the eighty
// or so call sites to pass it explicitly, and five of them did.
//
// So the request carries it instead. The middleware puts this on the request
// context once, and the audit repository reads it when an entry is appended.
// A caller that knows better still wins: anything it sets explicitly is kept.
package audit

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

// RequestContext is what the middleware knows about the caller.
type RequestContext struct {
	IPAddress         string
	UserAgent         string
	SessionID         string
	DeviceFingerprint string
	RequestID         string

	// Where the request was aimed. The audit schema has no column for a route,
	// so this is recorded in resource_attributes, which is where an inspection
	// asking "what did they open" will find it.
	Method string
	Route  string

	// The caller's place in the force, so an entry can be read years later
	// without joining to a users table whose rows have since been amended.
	ActorRole    string
	ActorStation *uuid.UUID

	// Coarse location derived from the address, when a resolver is configured.
	// Absent rather than guessed: an unknown location is recorded as unknown.
	GeoLocation []byte
}

// With returns a context carrying what is known about this request.
func With(ctx context.Context, rc *RequestContext) context.Context {
	if rc == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, rc)
}

// From returns what is known about the request, or nil outside one — a
// background job or a test writes an entry with no client address, and that is
// correct rather than missing.
func From(ctx context.Context) *RequestContext {
	if ctx == nil {
		return nil
	}
	rc, _ := ctx.Value(contextKey{}).(*RequestContext)
	return rc
}
