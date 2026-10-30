package main

import (
	"context"
	"testing"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// A notify URL the platform would refuse is the caller's validation error
// (400), found by Validate before anything runs or is written. The rows the
// operation writes are checked against embedded Postgres in the tests module
// (pkg/fcsdk/tests).
func TestIssueInvoiceRefusesABadNotifyURL(t *testing.T) {
	op := IssueInvoice(InvoiceRepository{})
	bad := IssueInvoiceCommand{Customer: "Acme", ClientCode: "acme", AmountCents: 1, NotifyURL: "ftp://acme.example.com"}
	err := op.Validate(context.Background(), bad)
	if ue := usecase.AsError(err); ue == nil || ue.Code != "NOTIFY_INVALID" || ue.HTTPStatus() != 400 {
		t.Fatalf("bad notify URL: %v", err)
	}
	good := IssueInvoiceCommand{Customer: "Acme", ClientCode: "acme", AmountCents: 4250, NotifyURL: "https://acme.example.com/hooks"}
	if err := op.Validate(context.Background(), good); err != nil {
		t.Fatal(err)
	}
}
