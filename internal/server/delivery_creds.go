package server

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// deliveryCredsDeps are the lookups newDeliveryCredsResolver walks —
// function-typed so the resolution order can be tested against fakes,
// without a database. wire_public.go hands in the repositories' methods.
type deliveryCredsDeps struct {
	subscription func(ctx context.Context, id string) (*subscription.Subscription, error)
	connection   func(ctx context.Context, id string) (*connection.Connection, error)
	application  func(ctx context.Context, code string) (*application.Application, error)
	// byServiceAccountID / byApplicationID are the cached credential lookups
	// (serviceaccount.NewCachedOutboundCredsByIDResolver / ...Resolver).
	byServiceAccountID func(ctx context.Context, id string) (serviceaccount.OutboundCreds, error)
	byApplicationID    func(ctx context.Context, id string) (serviceaccount.OutboundCreds, error)
}

// newDeliveryCredsResolver resolves the credentials (bearer token + HMAC
// signing secret) a dispatch job's delivery is stamped with.
//
// Owner ruling 2026-09-22, REVERSING 2026-09-19's "credentials belong to the
// application": the service account an operator configured on the
// subscription's CONNECTION is the one that signs. That is the account the
// connection form requires, and the subscription form asks for no
// application at all — so keying on the application meant two UIs describing
// a credential nothing read, and a Laravel subscriber holding the connection
// account's secret rejecting every delivery as unsigned-by-the-wrong-key.
//
// Order — the first that NAMES an account decides, with no fall-through past
// it. An account someone configured explicitly must be the one used, or
// declined with a reason; silently signing with some other account instead
// is how a rotated or deactivated credential keeps working by accident:
//
//  1. the subscription's own serviceAccountId (an explicit override);
//  2. the subscription's connection's serviceAccountId — the normal case;
//  3. the application's oldest active service account, found through the
//     subscription's applicationCode or, for a direct job with no
//     subscription, the job code's first segment (`billing:invoice:created`
//     → `billing`). This is the pre-ruling rule, kept as the fallback for
//     jobs that have no connection to name an account;
//  4. nothing — the delivery goes out bare, and OutboundCreds.Reason says
//     why, which the processing handler records on the attempt.
//
// A lookup error propagates: the caller degrades that to a bare delivery
// with a WARN (it must never abort a delivery), and the next attempt retries
// the lookup.
func newDeliveryCredsResolver(d deliveryCredsDeps) dispatchprocessing.DeliveryCredsResolver {
	return func(ctx context.Context, job *dispatchjob.DispatchJob) (serviceaccount.OutboundCreds, error) {
		var sub *subscription.Subscription
		if job.SubscriptionID != nil && *job.SubscriptionID != "" {
			var err error
			if sub, err = d.subscription(ctx, *job.SubscriptionID); err != nil {
				return serviceaccount.OutboundCreds{}, err
			}
		}

		if sub != nil {
			// 1. The subscription names its own account.
			if id := deref(sub.ServiceAccountID); id != "" {
				return named(ctx, d, "subscription "+sub.Code, id)
			}
			// 2. Its connection names one.
			if connID := deref(sub.ConnectionID); connID != "" {
				conn, err := d.connection(ctx, connID)
				if err != nil {
					return serviceaccount.OutboundCreds{}, err
				}
				if conn != nil && strings.TrimSpace(conn.ServiceAccountID) != "" {
					return named(ctx, d, "connection "+conn.Code, conn.ServiceAccountID)
				}
			}
		}

		// 3. The application's oldest active account.
		appCode := ""
		if sub != nil {
			appCode = deref(sub.ApplicationCode)
		}
		if appCode == "" {
			if seg, _, ok := strings.Cut(job.Code, ":"); ok && seg != "" {
				appCode = seg
			}
		}
		if appCode == "" {
			return serviceaccount.OutboundCreds{
				Reason: "no subscription, connection or application names a service account",
			}, nil
		}
		app, err := d.application(ctx, appCode)
		if err != nil {
			return serviceaccount.OutboundCreds{}, err
		}
		if app == nil {
			return serviceaccount.OutboundCreds{Reason: "application " + appCode + " does not exist"}, nil
		}
		creds, err := d.byApplicationID(ctx, app.ID)
		if err != nil {
			return serviceaccount.OutboundCreds{}, err
		}
		if creds.Empty() && creds.Reason == "" {
			creds.Reason = "application " + appCode + " has no service account credentials"
		}
		return creds, nil
	}
}

// named resolves an explicitly configured account. Whatever the answer —
// credentials, inactive, missing, no secrets — it is final: `who` configured
// this account, so the reason names `who`.
func named(ctx context.Context, d deliveryCredsDeps, who, serviceAccountID string) (serviceaccount.OutboundCreds, error) {
	creds, err := d.byServiceAccountID(ctx, serviceAccountID)
	if err != nil {
		return serviceaccount.OutboundCreds{}, err
	}
	if creds.Empty() {
		reason := creds.Reason
		if reason == "" {
			reason = "service account " + serviceAccountID + " has no webhook credentials"
		}
		creds.Reason = who + ": " + reason
	}
	return creds, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
