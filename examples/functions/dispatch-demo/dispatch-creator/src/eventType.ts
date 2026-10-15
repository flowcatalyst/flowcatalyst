// The demo event type: <application>:<subdomain>:<aggregate>:<event>. The
// application segment must be the code of the application that owns the
// dispatch-creator function (see the README); it is injected at build time
// (build.mjs, APP_CODE) and falls back to "dispatchdemo" under plain Node.
declare const __APP_CODE__: string | undefined;

const app = typeof __APP_CODE__ === "string" ? __APP_CODE__ : "dispatchdemo";

export const DISPATCH_REQUESTED = `${app}:demo:dispatch:requested`;

// The permission a caller must hold to use dispatch-creator, a 4-segment
// <application>:<context>:<resource>:<action> code like every platform
// permission. It travels in a role of the application (README), so it
// shows up in the caller's verified `permissions`.
export const REQUIRED_PERMISSION = `${app}:dispatch:job:create`;
