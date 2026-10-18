package main

// Everything you rename for a new function lives here. Replace every "changeme".

const (
	// Code of the FlowCatalyst application that owns BOTH functions of the pair.
	// The target function must be in this application AND the same client.
	appCode = "changeme"

	// Name (not address) of the webhook function the jobs are delivered to; its
	// address is <appCode>.<name>. It is any function with a webhook endpoint at
	// deliverPath, for example payload-logger from the dispatch-demo examples.
	targetFunctionName = "changeme-processor"
	targetAddress      = appCode + "." + targetFunctionName
	deliverPath        = "/deliver"

	// Dispatch pool and connection every job created by fn.Dispatch goes through
	// (declared with fn.Dispatches; a call cannot choose). Codes, never ids. The
	// connection is required and needs a service account that can sign; the pool
	// is optional (drop fn.DispatchPool from the declaration to use none). Both
	// must exist on the platform before the first dispatch. Pausing the
	// connection holds the jobs.
	dispatchPool       = "changeme-pool"
	dispatchConnection = "changeme-connection"

	// This function's public endpoint: POST <runner or domain>/fn/<address>/ingest
	ingestPath = "/ingest"

	// The shared secret the sender signs with, a declared secret of this function
	// (set it with `fcdev fn set <address> --secret INGEST_SIGNING_SECRET=...`).
	signingSecretKey = "INGEST_SIGNING_SECRET"
	// The header carrying "sha256=" + hex(HMAC-SHA256(secret, raw body)).
	signatureHeader = "X-Signature"

	// Shown on the dispatch job. A job's message group also orders delivery: jobs
	// with the same group are delivered one at a time, in order. The sender may
	// pick one with X-Message-Group; this is the default.
	defaultMessageGroup = "changeme-ingest"
	descriptor          = "changeme webhook payload"
)
