// The demo event type: <application>:<subdomain>:<aggregate>:<event>. The
// application segment must be the code of the application that owns the
// dispatch-creator function (see the README); it is injected at build time
// (build.mjs, APP_CODE) and falls back to "dispatchdemo" under plain Node.
declare const __APP_CODE__: string | undefined;

const app = typeof __APP_CODE__ === "string" ? __APP_CODE__ : "dispatchdemo";

export const DISPATCH_REQUESTED = `${app}:demo:dispatch:requested`;
