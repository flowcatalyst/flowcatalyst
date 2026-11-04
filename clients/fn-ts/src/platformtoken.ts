// Platform token (plan op 15, platform.token; docs/function-platform-token.md),
// mirroring clients/fn-go/platformtoken.go. Gives the function a FlowCatalyst
// access token to send as a bearer to another first-party application that
// accepts FlowCatalyst tokens. The function holds no client id or secret: the
// host mints the token as the service account linked to the function's
// application, caches it and renews it before it expires.
import { registry } from "./registry";
import { callHost } from "./native";
import { OpPlatformToken } from "./ops";
import { ErrCodeUnknownOp, FnError } from "./errors";

export interface PlatformTokenOptions {
  /** Skip the host's cached token and fetch a new one. Use it when the target
   * answered 401 with the token you were given (for example the account's
   * roles changed). */
  refresh?: boolean;
}

/** Declares that this function calls platformToken() (describe key
 * `platformToken: true`). Call at module top level. Without it the host
 * refuses the call with NOT_DECLARED. */
export function usesPlatformToken(): void {
  registry.platformToken = true;
}

/** Returns a FlowCatalyst access token. The host caches it, so call it per
 * request rather than keeping the token. Never log it. Throws FnError;
 * isRetryable(err) is true for UNAVAILABLE (platform trouble). NOT_DECLARED
 * (usesPlatformToken() not called), NOT_ALLOWED (the platform refused: not
 * declared there, the application has no linked service account, or the
 * account is inactive; the message carries the platform code) and DEADLINE
 * are final. A runner that predates the capability answers UNKNOWN_OP, thrown
 * as a FnError with that code and a message saying the runner does not
 * support platform tokens. */
export async function platformToken(options: PlatformTokenOptions = {}): Promise<string> {
  const meta: { refresh?: boolean } = {};
  if (options.refresh) meta.refresh = true;
  let rm: Record<string, unknown>;
  try {
    rm = callHost(OpPlatformToken, meta).meta;
  } catch (err) {
    if (err instanceof FnError && err.code === ErrCodeUnknownOp) {
      throw new FnError(
        ErrCodeUnknownOp,
        "this runner does not support platform tokens (it predates host op 15, platform.token); upgrade the function runner",
      );
    }
    throw err;
  }
  if (typeof rm.accessToken !== "string" || rm.accessToken === "") {
    throw new Error("fn: platform.token: host returned no access token");
  }
  return rm.accessToken;
}
