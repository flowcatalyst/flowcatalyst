package main

// Names the demo shares between its two functions. appCode must be the code of
// the application that owns BOTH functions (see the README): edit it before you
// build ("example" on the owner's fcdev). It is a constant, not an environment
// variable, because `fcdev fn build` passes no linker flags.
const appCode = "dispatchdemo"

const (
	// The function the jobs are delivered to, and the webhook path on it that
	// payload-logger declares. A function may only dispatch to a function of its
	// own application, so the address is built from the same application code.
	loggerAddress = appCode + ".payload-logger"
	deliverPath   = "/deliver"

	// The connection the jobs are delivered through (required by every
	// dispatches declaration). Create it once (README step 0) with the
	// application's own service account, so the signature it puts on a delivery
	// is the one payload-logger's webhook endpoint verifies. Pausing it holds
	// the jobs PENDING until it is reactivated.
	connectionCode = "dispatch-demo-conn"

	// The permission a caller must hold to use dispatch-creator: a 4-segment
	// <application>:<context>:<resource>:<action> code like every platform
	// permission, carried by a role of the application (README).
	requiredPermission = appCode + ":dispatch:job:create"
)
