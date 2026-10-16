// Names the demo shares between its two functions. The application segment
// must be the code of the application that owns BOTH functions (see the
// README); it is injected at build time (build.mjs, APP_CODE) and falls back
// to "dispatchdemo" under plain Node.
declare const __APP_CODE__: string | undefined;

const app = typeof __APP_CODE__ === "string" ? __APP_CODE__ : "dispatchdemo";

// The function the jobs are delivered to, and the webhook path on it that
// payload-logger declares. A function may only dispatch to a function of its
// own application, so the address is built from the same application code.
export const LOGGER_ADDRESS = `${app}.payload-logger`;
export const DELIVER_PATH = "/deliver";

// The connection the jobs are delivered through (required by every
// dispatches declaration). Create it once (README step 0) with the
// application's own service account, so the signature it puts on a delivery
// is the one payload-logger's webhook endpoint verifies. Pausing it holds the
// jobs PENDING until it is reactivated.
export const CONNECTION_CODE = "dispatch-demo-conn";

// The permission a caller must hold to use dispatch-creator, a 4-segment
// <application>:<context>:<resource>:<action> code like every platform
// permission. It travels in a role of the application (README), so it
// shows up in the caller's verified `permissions`.
export const REQUIRED_PERMISSION = `${app}:dispatch:job:create`;
