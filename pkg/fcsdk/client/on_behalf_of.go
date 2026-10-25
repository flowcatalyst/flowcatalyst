package client

import (
	"context"
	"errors"
	"strings"
)

// OnBehalfOfHeader names the person a service account acts for. The platform
// records it in the audit row of a platform-config set, delete or reveal, and
// honours it only from service accounts.
const OnBehalfOfHeader = "X-On-Behalf-Of"

// maxOnBehalfOf is the platform's limit on the header (longer is a 400).
const maxOnBehalfOf = 256

// ErrInvalidOnBehalfOf is returned before anything is sent when the value
// given to WithOnBehalfOf is too long or holds a control character.
var ErrInvalidOnBehalfOf = errors.New("fcsdk: on-behalf-of must be at most 256 characters with no control characters")

type onBehalfOfKey struct{}

// WithOnBehalfOf returns a context whose calls send X-On-Behalf-Of: who (an
// id or an email, never a token), so the platform's audit row names the
// person a service account acts for:
//
//	ctx = client.WithOnBehalfOf(ctx, user.ID)
//	c.PlatformConfig().Set(ctx, "orders", "ftp", "password", req)
//
// A blank who sends nothing. The platform uses it on platform-config set,
// delete and reveal; other endpoints ignore it.
func WithOnBehalfOf(ctx context.Context, who string) context.Context {
	return context.WithValue(ctx, onBehalfOfKey{}, strings.TrimSpace(who))
}

func onBehalfOfFrom(ctx context.Context) (string, error) {
	who, _ := ctx.Value(onBehalfOfKey{}).(string)
	if who == "" {
		return "", nil
	}
	if len(who) > maxOnBehalfOf || strings.ContainsFunc(who, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", ErrInvalidOnBehalfOf
	}
	return who, nil
}
