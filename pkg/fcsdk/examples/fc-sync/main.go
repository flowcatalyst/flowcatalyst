// Command fc-sync is the declarative-reconciliation example. It builds
// a DefinitionSet describing what one application "owns" (roles, event
// types, dispatch pools, connections, subscriptions, processes, scheduled
// jobs, principals, documentation pages), then hands it to a Synchronizer
// that calls the per-category sync endpoints in dependency order.
//
// One platform call per category (per client for scheduled jobs).
// RemoveUnlisted=true asks the platform to archive SDK-sourced rows that
// aren't in this set; UI-sourced rows are never touched. Documentation is
// always a full replace.
//
// # Run
//
//	FC_BASE_URL=https://api.flowcatalyst.io  \
//	FC_TOKEN=eyJ...                          \
//	go run ./examples/fc-sync
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/client"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/sync"
)

const appCode = "orders"

func main() {
	c := client.New(
		mustEnv("FC_BASE_URL"),
		client.WithToken(mustEnv("FC_TOKEN")),
	)

	set := sync.ForApplication(appCode).
		// Roles — Name is the short form; the platform prefixes it with
		// the app code (orders:admin). Permissions are four-segment
		// {app}:{context}:{aggregate}:{action}.
		AddRole(sync.MakeRole("admin").
			WithDisplayName("Orders Admin").
			WithDescription("Full read/write access to orders.").
			WithPermissions("orders:sales:order:view", "orders:sales:order:manage")).
		AddRole(sync.MakeRole("viewer").
			WithDisplayName("Orders Viewer").
			WithPermissions("orders:sales:order:view").
			ClientManagedEnabled()).

		// Event types — Code is the full four-segment identifier
		// {app}:{domain}:{aggregate}:{event}.
		AddEventType(sync.MakeEventType(
			"orders:sales:order:placed", "Order placed").
			WithDescription("A new order has been accepted.")).
		AddEventType(sync.MakeEventType(
			"orders:sales:order:cancelled", "Order cancelled")).

		// Dispatch pools — concurrency (and optional rate) control for
		// downstream delivery. Synced before subscriptions that name them.
		AddDispatchPool(sync.MakeDispatchPool("orders-default", "Orders pool").
			WithConcurrency(8).
			WithRateLimit(600). // messages per minute
			WithDescription("Catch-all for orders subscriptions.")).

		// Connections — what delivers a subscription. Nothing
		// environment-specific: the platform assigns the application's own
		// service account. Synced before subscriptions.
		AddConnection(sync.MakeConnection("orders-billing", "Billing webhooks")).

		// Subscriptions — bind event types to a webhook target, delivered
		// through a connection named by code (stable across environments).
		AddSubscription(sync.MakeSubscription(
			"order-billing", "Forward placed orders to billing",
			"https://billing.example.com/webhooks/orders").
			WithConnectionCode("orders-billing").
			WithDispatchPool("orders-default").
			WithMode("IMMEDIATE").
			WithMaxRetries(5).
			Bind("orders:sales:order:placed", "")).

		// Processes — documentation (no runtime effect): a Mermaid diagram.
		AddProcess(sync.MakeProcess("orders:sales:place-order", "Place Order").
			WithDescription("End-to-end flow for placing an order.").
			WithBody("graph LR; Cart-->Checkout-->Placed").
			WithTags("sales")).

		// Scheduled jobs — six-field, seconds-first crons; the platform
		// fires them at TargetURL (or the application's default).
		AddScheduledJob(sync.MakeScheduledJob("orders:nightly-reconcile", "Nightly reconcile", "0 0 2 * * *").
			WithTimezone("Europe/London").
			WithTargetURL("https://orders.example.com/jobs")).

		// Documentation pages — shown to administrators; a full replace.
		AddDoc(sync.MakeDoc("getting-started", "# Getting started\n\nHow the orders application uses FlowCatalyst.\n"))

	r := sync.NewSynchronizer(c).Sync(
		context.Background(),
		set,
		sync.WithRemoveUnlisted(), // archive SDK-sourced rows not in the set
	)

	printSummary(r)
	if r.HasErrors() {
		os.Exit(1)
	}
}

func printSummary(r *sync.Result) {
	fmt.Printf("application=%s\n", r.ApplicationCode)
	for _, cat := range []struct {
		name string
		res  *sync.CategoryResult
	}{
		{"roles", r.Roles},
		{"event_types", r.EventTypes},
		{"dispatch_pools", r.DispatchPools},
		{"connections", r.Connections},
		{"subscriptions", r.Subscriptions},
		{"processes", r.Processes},
		{"scheduled_jobs", r.ScheduledJobs},
		{"principals", r.Principals},
		{"openapi", r.OpenAPI},
		{"docs", r.Docs},
	} {
		if cat.res == nil {
			fmt.Printf("  %-15s skipped\n", cat.name)
			continue
		}
		if cat.res.IsError() {
			fmt.Printf("  %-15s ERROR: %s\n", cat.name, cat.res.Error)
			continue
		}
		fmt.Printf("  %-15s created=%d updated=%d deleted=%d\n",
			cat.name, cat.res.Created, cat.res.Updated, cat.res.Deleted)
		if len(cat.res.SkippedRoles) > 0 {
			fmt.Printf("  %-15s skipped unknown roles: %v\n", "", cat.res.SkippedRoles)
		}
	}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
