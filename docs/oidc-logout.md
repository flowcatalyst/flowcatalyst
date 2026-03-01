# OIDC Logout — RP-Initiated Logout

## The Problem

When a user logs out of your app, simply clearing your local session is not enough. The FlowCatalyst OIDC provider maintains its own server-side session (`fc_session` cookie scoped to the FlowCatalyst domain). If that session is not ended, the next time your app starts an OIDC authorization flow the provider will silently re-authenticate the user using the existing session — bypassing the login prompt entirely.

This is by design in OIDC (SSO behaviour), but it means your logout button will appear broken: the user logs out and immediately gets logged back in as the same user.

## The Solution — RP-Initiated Logout

Instead of clearing only your local tokens, redirect the user to FlowCatalyst's `end_session_endpoint`. This is the standard OIDC [RP-Initiated Logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html) flow.

FlowCatalyst's end session endpoint:

```
GET {FLOWCATALYST_ISSUER}/session/end
```

### Required / Recommended Parameters

| Parameter | Required | Description |
|---|---|---|
| `id_token_hint` | Recommended | The ID token your app received during login. Tells the provider which session to end. |
| `post_logout_redirect_uri` | Recommended | Where to redirect the user after logout completes. Must be pre-registered on the client. |
| `state` | Optional | Random value echoed back in the redirect, use for CSRF protection. |

### Example

```
GET https://auth.yourcompany.com/session/end
  ?id_token_hint=eyJhbGciOiJSUzI1NiJ9...
  &post_logout_redirect_uri=https://yourapp.com/logged-out
  &state=abc123
```

## Logout Flow

```
1. User clicks logout in your app
2. Your app clears its local session / tokens
3. Your app redirects the browser to FlowCatalyst /session/end (with params above)
4. FlowCatalyst clears the fc_session cookie on its domain
5. FlowCatalyst destroys the OIDC session and any associated grants
6. FlowCatalyst redirects the browser to your post_logout_redirect_uri
7. Your app shows a "you have been logged out" page
```

Steps 1–3 happen in your app. Steps 4–6 happen on the FlowCatalyst domain. This is why the browser redirect is necessary — only the FlowCatalyst domain can clear its own cookie.

## Common Mistake — Local-Only Logout

```
// ❌ Wrong — only clears the app's own session
async function logout() {
  await clearLocalTokens();
  router.push("/login");
}
```

```
// ✅ Correct — also ends the provider session
async function logout() {
  const idToken = getStoredIdToken();
  await clearLocalTokens();

  const params = new URLSearchParams({
    id_token_hint: idToken,
    post_logout_redirect_uri: "https://yourapp.com/logged-out",
    state: crypto.randomUUID(),
  });

  window.location.href =
    `${FLOWCATALYST_ISSUER}/session/end?${params.toString()}`;
}
```

## Localhost / Development

This applies equally in development. If your app runs on `localhost` and points at a deployed FlowCatalyst instance, the `fc_session` cookie lives on the FlowCatalyst domain — not localhost. Clearing cookies on localhost has no effect on it.

You must still redirect to `/session/end` to clear it. Make sure `http://localhost:{port}/logged-out` (or whatever your local redirect URI is) is registered as an allowed `post_logout_redirect_uri` on the OIDC client in FlowCatalyst.

### Debugging — Quick Manual Clear

If you are stuck with the wrong user in development, open DevTools on the FlowCatalyst domain and delete the `fc_session` cookie manually, or log in to FlowCatalyst directly as the correct user first.

## `post_logout_redirect_uri` Registration

The redirect URI after logout must be registered on the OIDC client in FlowCatalyst. Unregistered URIs will be rejected and the user will see an error instead of being redirected.

Register both your production and development URIs on the client:

```
https://yourapp.com/logged-out
http://localhost:3000/logged-out
```

## What FlowCatalyst Does on /session/end

1. Validates the `id_token_hint` and resolves the session
2. Clears the `fc_session` cookie on the FlowCatalyst domain
3. Destroys the OIDC session record and all associated grants/tokens in the database
4. Redirects to `post_logout_redirect_uri` if provided and registered

The logout is immediate and complete. Any subsequent authorization request from your app will require the user to authenticate again.
