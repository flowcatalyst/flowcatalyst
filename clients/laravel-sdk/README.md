# FlowCatalyst Laravel SDK

Official Laravel SDK for the [FlowCatalyst](https://flowcatalyst.io) platform.

Use FlowCatalyst as your app's **identity provider and authorization layer**, and
talk to the **event-driven control plane** — all from idiomatic Laravel.

**What's in the box**

- **Sign in with FlowCatalyst (OIDC)** — stock `->middleware('auth')` drives the
  login flow; guests are redirected, users come back authenticated. No scaffolding.
- **Authorization that feels like Spatie** — `$user->can(...)`, `@can`, `hasRole()`,
  policies — backed by FlowCatalyst roles/permissions. Run it **stateless** (no
  users table, no Spatie) or sync into a local `users` table + `spatie/laravel-permission`.
- **API auth** — validate `client_credentials` / user Bearer tokens with the same
  permission surface (`auth:fc-token`).
- **Declare & sync your RBAC** — define roles and permissions as PHP classes
  (`#[AsRole]`, `#[AsPermission]`) and push them to the platform.
- **Outbox** — create events/dispatch jobs via the transactional outbox.
- **Control plane client** — typed access to event types, subscriptions, dispatch
  pools, applications, roles, and more.
- **Webhook validation** — verify inbound HMAC-signed deliveries.

## Requirements

- PHP 8.1+
- Laravel 10, 11, 12, or 13
- For the Outbox: MySQL 8.0+ / PostgreSQL 12+ / MongoDB 4.4+
- For local authorization mirroring (optional): `spatie/laravel-permission`

## Contents

- [Installation](#installation) · [Local dev with `fcdev`](#local-development-with-fcdev)
- [Authentication & authorization](#authentication--authorization) — login, guards, permissions, modes, refresh
- [Declaring & syncing roles and permissions](#declaring--syncing-roles-and-permissions)
- [Control plane API](#control-plane-api) · [Portal identities](#portal-identities) · [Field workers](#field-workers) · [Outbox](#outbox-event-creation) · [Push to clients](#push-to-clients) · [Webhooks](#webhook-validation)

## Installation

The SDK is not on Packagist. Add the split repository,
[flowcatalyst/laravel-sdk](https://github.com/flowcatalyst/laravel-sdk), as a
Composer `vcs` repository, then require it (releases are the repository's
`vX.Y.Z` tags):

```bash
composer config repositories.flowcatalyst vcs https://github.com/flowcatalyst/laravel-sdk
composer require flowcatalyst/laravel-sdk
```

Publish the configuration file:

```bash
php artisan vendor:publish --tag=flowcatalyst-config
```

## Local development with `fcdev`

For local work you need a FlowCatalyst control plane to talk to.
`fcdev` is the official one-binary dev environment — bundled
PostgreSQL, platform API, message router, scheduler, and frontend
in a single process.

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex

fcdev          # starts API on http://localhost:8080
```

If you use the **Outbox** (outbox pattern), you also need
`fcdev outbox` running as a sidecar — the SDK only writes rows to
your app's `outbox_messages` table; `fcdev outbox` is what
forwards them to the platform.

```bash
# In your Laravel project directory:

# Once (optional): create the `outbox_messages` table. `fcdev outbox`
# also creates it on first run for Postgres; use this for MySQL/MongoDB or
# to provision it ahead of time. Postgres shown:
fcdev outbox create-table --db-type pg --db-url postgres://user:pass@localhost:5432/app

# Run the poller. It loads ./.env (override with --env-file) and reads
# FC_OUTBOX_SOURCE_DB_URL (your app database), FC_OUTBOX_PLATFORM_URL and
# the service-account credentials FLOWCATALYST_CLIENT_ID /
# FLOWCATALYST_CLIENT_SECRET (or FC_OUTBOX_PLATFORM_AUTH_TOKEN for a static
# token). Existing environment variables win over .env.
fcdev outbox
```

If your application creates the table with its own migrations (the SDK's
opt-in migration, published with
`php artisan vendor:publish --tag=flowcatalyst-migrations` and run by
`php artisan migrate`), run the poller with `FC_OUTBOX_INIT_SCHEMA=false`
(or `fcdev outbox --init-schema=false`). The migration is a plain
`Schema::create`: a poller that starts before it, such as a second container
started with the application, would create the table first and the migration
would fail with "already exists" on every deploy. With schema creation off the
poller runs no DDL, waits for the migration to create the table, then starts.

Complete reference: run `fcdev --help`, or see the
[FlowCatalyst repository](https://github.com/flowcatalyst/flowcatalyst).

## Configuration

Add the following to your `.env` file:

```env
# FlowCatalyst API
FLOWCATALYST_BASE_URL=https://your-instance.flowcatalyst.io
FLOWCATALYST_CLIENT_ID=your_client_id
FLOWCATALYST_CLIENT_SECRET=your_client_secret

# Outbox (for event creation)
FLOWCATALYST_OUTBOX_DRIVER=database
# Off by default: an outbox write outside a DB transaction only logs a warning.
# Set true to make the database driver throw instead.
FLOWCATALYST_OUTBOX_STRICT_TRANSACTIONS=false

# Webhook validation (optional)
FLOWCATALYST_SIGNING_SECRET=your_signing_secret
```

## Authentication & authorization

The SDK does two things: **signs users in** against FlowCatalyst's OIDC server,
and lets you **authorize** them with a Spatie-style surface (`can`, `@can`,
`hasRole`, policies) backed by FlowCatalyst roles/permissions.

### Quick start — sign in with FlowCatalyst

A fresh Laravel app needs only env. Stock `->middleware('auth')`, `Auth::user()`,
`@auth`, and policies all recognise the signed-in user.

```env
FLOWCATALYST_BASE_URL=https://your-instance.flowcatalyst.io
FLOWCATALYST_OIDC_ENABLED=true
FLOWCATALYST_NATIVE_LOGIN=true   # the zero-code bridge: local-user upsert + guest redirect
FLOWCATALYST_OIDC_CLIENT_ID=your_oauth_client_id
FLOWCATALYST_OIDC_CLIENT_SECRET=your_oauth_client_secret   # omit for a PKCE-only public client
FLOWCATALYST_OIDC_SCOPE="openid profile email offline_access"   # offline_access → refresh tokens
```

Protect routes the normal Laravel way:

```php
// routes/web.php
Route::middleware('auth')->group(function () {
    Route::get('/dashboard', DashboardController::class);
});
```

That's it. With the native-login bridge on (`FLOWCATALYST_NATIVE_LOGIN=true` —
it's opt-in so the SDK never changes an existing app's auth behaviour), a guest
hitting an `auth` route is **automatically redirected into the FlowCatalyst
login flow** — no `login` route or custom redirect needed. (If your app defines
its own `login` route, that one wins; the SDK won't hijack a local login page.
Opt out of just the redirect with `FLOWCATALYST_OIDC_AUTO_GUEST_REDIRECT=false`.)

The routes `/flowcatalyst/{login,callback,logout,refresh}` are registered for
you. On your FlowCatalyst OAuth client, set the grant to `authorization_code`
and the redirect URI to **`https://your-app.com/flowcatalyst/callback`** (it must
match exactly — host, port, and scheme — including `localhost` vs `127.0.0.1`).

The logout route accepts GET and POST by default. A GET logout can be triggered
cross-site (an `<img>` or link), so we recommend `'oidc' => ['logout_methods' => ['post']]`
in `config/flowcatalyst.php` and logging out through a CSRF-protected form POST.
The session id is regenerated at sign-in (session fixation defence).

### Checking permissions

Once authenticated, the user (whether resolved from a browser session or a
Bearer token) speaks the Spatie vocabulary — FlowCatalyst wildcards honoured:

```php
$user->hasPermissionTo('orders:admin:order:cancel');
$user->hasRole('orders:admin');
$user->can('orders:admin:order:cancel');           // Gate, @can, policies route here

// Blade
@can('orders:admin:order:cancel')
    <button>Cancel order</button>
@endcan
```

Permission strings are 4-part `application:context:aggregate:action`; role names
are platform-prefixed (`orders:admin`).

### Two modes

| | **Stateless** (default) | **Local sync** |
|---|---|---|
| Local `users` row | none | upserted on login |
| `spatie/laravel-permission` | not needed | used (roles/permissions mirrored) |
| Permissions come from | the token's `scope` claim | local Spatie tables |
| `Auth::user()` guard | `fc-session` / `fc-token` | native `web` |
| Enable | `FLOWCATALYST_OIDC_HANDLER=session` | `FLOWCATALYST_OIDC_HANDLER=database` |

**Stateless** is the leanest: no migrations, no Spatie, nothing to keep in sync —
the access token already carries the granted permissions. **Local sync** is for
apps that want a real `users` row (to foreign-key against) and to manage
authorization through Spatie locally.

#### Stateless (no users table, no Spatie)

```env
FLOWCATALYST_OIDC_HANDLER=session
FLOWCATALYST_OIDC_PERMISSION_RESOLVER=token   # read permissions off the token scope (default)
```

Authorize via the FlowCatalyst guards:

- **Browser / OIDC login:** `->middleware('auth:fc-session')` (or set `fc-session`
  as your app's default guard so bare `auth` uses it).
- **API / Bearer** (user *and* `client_credentials` tokens): `->middleware('auth:fc-token')`.

The Bearer guard refuses an OIDC **ID token** presented as an API credential: a
token with no `token_use` that carries `nonce`, `auth_time` or `at_hash`, or whose
`azp` equals its single `aud`, gets a 401 (platform ID tokens are aimed at the
OAuth client and carry the user's roles, so another relying party must not be able
to replay one here). Access tokens (`token_use` `api` / `identity`) are unaffected.
`FLOWCATALYST_OIDC_ACCEPT_ID_TOKEN_AS_BEARER=true` (config
`flowcatalyst.oidc.accept_id_token_as_bearer`, default `false`) restores the old
behaviour for an integration that still sends ID tokens as bearers.

Both resolve a `FlowCatalystAuthenticatable` — read-only, no DB. Set
`FLOWCATALYST_OIDC_PERMISSION_RESOLVER=api_me` to resolve via `/api/me` instead
(one cached HTTP call, fresher on changes), or bind an offline `RbacCatalogue`.

> **Policies: don't type-hint your Eloquent user.** `FlowCatalystAuthenticatable`
> is not `App\Models\User`, so a policy method signed
> `view(User $user, Order $order)` never matches a token/session principal —
> Laravel skips the method and the check **silently denies**. Type-hint the
> contract (or nothing) in policies that must run for FlowCatalyst identities:
>
> ```php
> public function view(Authenticatable $user, Order $order): bool
> {
>     return $user->hasPermissionTo('orders:admin:order:view');
> }
> ```
>
> (In **local-sync** mode this doesn't apply — there `Auth::user()` IS your
> Eloquent model.) Spatie's own `role:`/`permission:` route middleware work
> unchanged with either mode.

#### Local sync (users table + Spatie)

```env
FLOWCATALYST_OIDC_HANDLER=database
FLOWCATALYST_OIDC_USER_MODEL="App\Models\User"   # upserted by email on login
```

On login the SDK upserts the user, `Auth::login()`s them, and — if the model
uses Spatie's `HasRoles` trait — mirrors the token's roles **and each role's
permissions** into the local Spatie tables, so stock `auth` + `$user->can(...)`
work against a real record:

```env
FLOWCATALYST_OIDC_SYNC_ROLES=true
FLOWCATALYST_OIDC_SYNC_ROLES_MODE=additive          # or "replace" for authoritative
FLOWCATALYST_OIDC_SYNC_ROLE_PERMISSIONS=true        # pull each role's permissions too
FLOWCATALYST_OIDC_CREATE_MISSING_ROLES=false        # create local roles on the fly
FLOWCATALYST_OIDC_ROLES_GUARD=web
```

### Middleware reference

The service provider registers these aliases (no app wiring needed):

| Middleware | Behaviour |
|---|---|
| `auth:fc-token` | Authenticate a FlowCatalyst Bearer token (user or `client_credentials`); 401 otherwise. Stack guards to fall through: `auth:fc-token,api`. |
| `auth:fc-session` | Authenticate the OIDC session principal; 401/redirect otherwise. |
| `fc.auth` | Resolve the principal (bearer wins over session) into `$request->attributes['fc.principal']` — never rejects. The three guards below only read that attribute, so they go **after** it: `['fc.auth', 'fc.bearer']`. |
| `fc.bearer` | 401 JSON when nothing resolved. For API routes. |
| `fc.session` | Redirect to `/flowcatalyst/login?returnTo=…` when nothing resolved. For web routes. |
| `fc.any` | Either mechanism; on miss, browser navigations redirect to login, everything else gets 401 JSON. |
| `fc.or-passport` | FlowCatalyst token **or** a legacy fallback chain (below). |

#### Migrating from Passport client-credentials

`fc.or-passport` lets one route accept FlowCatalyst service-account tokens
while still honouring your existing Passport machine tokens. A valid
FlowCatalyst token wins (the fallback never runs); anything else flows through
the configured chain exactly as if the middleware weren't there:

```php
// config/flowcatalyst.php
'oidc' => [
    'token_fallback_middleware' => ['client', 'passport.app.identity'],
],

Route::post('/api/stock-sync', $handler)->middleware('fc.or-passport');
```

An empty list means "FlowCatalyst-only": unauthenticated requests pass through
untouched, so a downstream guard decides. Once the last Passport caller is
migrated, swap the route to `auth:fc-token` and drop the fallback config.

### Keeping permissions fresh

In stateless mode permissions live in the (short-lived) access token, so they go
stale when roles change on the platform. `POST /flowcatalyst/refresh`
(`route('flowcatalyst.refresh')`) uses the stored refresh token to mint a fresh
access token and re-store the principal — wire it to a "Refresh" button:

```blade
<form method="POST" action="{{ route('flowcatalyst.refresh') }}">
    @csrf
    <button type="submit">Refresh permissions</button>
</form>
```

Requires a refresh token, so request `offline_access` in `FLOWCATALYST_OIDC_SCOPE`.
In local-sync mode, refresh also re-runs the role/permission sync.

### Configuration reference

| Env var | Default | Purpose |
|---|---|---|
| `FLOWCATALYST_OIDC_ENABLED` | `false` | turn the OIDC login + routes on |
| `FLOWCATALYST_NATIVE_LOGIN` | `false` | opt-in zero-code bridge: defaults the handler to `database` and enables the guest redirect |
| `FLOWCATALYST_OIDC_CLIENT_ID` / `_SECRET` | – | your OAuth login client (secret optional for PKCE) |
| `FLOWCATALYST_OIDC_SCOPE` | `openid profile email` | add `offline_access` for refresh tokens |
| `FLOWCATALYST_OIDC_HANDLER` | from bridge | `database` (users table) or `session` (stateless); unset → `database` when the bridge is on, else `session` |
| `FLOWCATALYST_OIDC_PERMISSION_RESOLVER` | `token` | `token` (scope claim) or `api_me` (`/api/me`) |
| `FLOWCATALYST_OIDC_AUTO_GUEST_REDIRECT` | `true` | redirect stock-`auth` guests into OIDC (only active with the bridge on) |
| `FLOWCATALYST_OIDC_PROVIDER` | – | `idp_...` id: send logins straight to a named upstream IdP (`?provider=` overrides per request) |
| `FLOWCATALYST_OIDC_CLIENT` | – | client identifier (slug) whose login branding the sign-in pages wear (`?client=` overrides per request); cosmetic only, ignored in portal mode |
| `FLOWCATALYST_OIDC_PORTAL` | `false` | portal mode: log in through `/portal/authorize` (see [Portal identities](#portal-identities)) |
| `FLOWCATALYST_OIDC_USER_MODEL` | `App\Models\User` | model upserted in `database` mode |
| `FLOWCATALYST_OIDC_SYNC_ROLES` / `_SYNC_ROLE_PERMISSIONS` | `true` | mirror roles / their permissions to Spatie |
| `FLOWCATALYST_OIDC_ROLES_GUARD` | `web` | Spatie guard for synced roles |
| `FLOWCATALYST_REDIRECT_AFTER_LOGIN` | `/dashboard` | post-login landing |

### Custom mapping

Need tenant checks, extra columns, or your own logic? Bind your own handler — it
always wins over the SDK default:

```php
// AppServiceProvider::register()
$this->app->bind(\FlowCatalyst\Auth\Contracts\OidcUserHandler::class, MyHandler::class);
```

Extend `DatabaseOidcUserHandler` to keep the upsert + login and override only
what you need, or implement `OidcUserHandler` from scratch.

The `DatabaseOidcUserHandler` + Spatie seeding/sync (`flowcatalyst:sync`,
`oidc.sync_role_permissions`) remain available for apps that *do* want a local
users table — when enabled, refresh re-runs the sync too.

## Declaring & syncing roles and permissions

Define your app's RBAC as PHP classes and push it to the platform — your repo is
the source of truth. FlowCatalyst has no standalone "create permission" endpoint,
so permissions reach the platform via the roles that grant them.

**Define** roles and reusable permissions in `app/FlowCatalyst`:

```php
// app/FlowCatalyst/Permissions/ViewPosts.php
use FlowCatalyst\Attributes\AsPermission;

// the application segment defaults to FLOWCATALYST_APP_CODE → "<app>:posts:post:view"
#[AsPermission(context: 'posts', aggregate: 'post', action: 'view', description: 'View posts')]
class ViewPosts {}
```

```php
// app/FlowCatalyst/Roles/EditorRole.php
use App\FlowCatalyst\Permissions\{EditPosts, ViewPosts};
use FlowCatalyst\Attributes\AsRole;

#[AsRole(name: 'editor', displayName: 'Editor', permissions: [ViewPosts::class, EditPosts::class])]
class EditorRole {}
```

A permission is declared once and **linked from many roles** by class. You can
also inline `new PermissionInput(...)` objects or plain `app:context:aggregate:action`
strings.

**Declare event types** by attributing your domain events, so the platform knows
about them (and they can be bound to subscribers and replayed):

```php
// app/UseCases/Comments/CommentCreated.php
use FlowCatalyst\Attributes\AsEventType;

// Synced code = "<app>:comments:comment:created" (the app prefix is added at sync).
// clientScoped: events of this type belong to a client.
#[AsEventType(subdomain: 'comments', aggregate: 'comment', event: 'created', name: 'Comment Created', clientScoped: true)]
final class CommentCreated extends BaseDomainEvent { /* … */ }
```

Make sure the directory holding them is listed in `flowcatalyst.definitions.paths`
(e.g. add `app_path('UseCases')` alongside the default `app_path('FlowCatalyst')`).

**Sync** with a service account (separate from your login client) that has
roles-write on the application:

```env
FLOWCATALYST_CLIENT_ID=svc_...        # client_credentials creds
FLOWCATALYST_CLIENT_SECRET=...
FLOWCATALYST_APP_CODE=blog            # the application that owns these roles
```

```bash
php artisan flowcatalyst:scan            # find #[AsRole]/#[AsPermission] classes, cache them
php artisan flowcatalyst:sync --dry-run  # preview
php artisan flowcatalyst:sync            # push roles (with their permissions) to the platform
```

If `spatie/laravel-permission` is installed, `flowcatalyst:sync` **also seeds the
local Spatie tables** so your app's authorization matches what it pushed (toggle
with `FLOWCATALYST_SEED_SPATIE` / `--no-spatie`). The same attribute mechanism
covers `#[AsEventType]`, `#[AsSubscription]`, `#[AsConnection]`, `#[AsDispatchPool]`,
`#[AsProcess]`, and `#[AsScheduledJob]`. See [`docs/syncing-definitions.md`](docs/syncing-definitions.md)
for the full reference, including the programmatic `DefinitionSynchronizer`.

### Multiple applications in one codebase

`flowcatalyst:sync` resolves each definition's application and syncs each app
separately — you don't have to drop to the programmatic API. The application is
resolved in this order:

1. an explicit `application:` on the attribute — `#[AsScheduledJob(application: 'orders', …)]`.
   Every definition attribute accepts it;
2. a longest-prefix match in the `definitions.application_map` config;
3. the default `--app` / `FLOWCATALYST_APP_CODE`.

The **map** is ideal when one codebase owns several apps, or when a package ships
its own definitions and is its own application — the *consumer* maps the package's
namespace to whatever app code they registered (a package author can't hardcode it):

```php
// config/flowcatalyst.php → definitions
'application_map' => [
    'App\\Orders\\'       => 'orders',
    'Vendor\\Logistics\\' => 'logistics',   // a package = its own application
],
```

`scan` records the resolved app per definition; `sync` groups by it and pushes
each application's catalog to its own `…/applications/{appCode}/…/sync` endpoint.

Note that `--remove-unlisted` is full-replace *per application, per category*,
and an application is only contacted when at least one definition resolves to
it. Moving the last definition of a category out of an app therefore leaves the
old rows in place rather than pruning them — see
[`docs/syncing-definitions.md`](docs/syncing-definitions.md).

## Control Plane API

The SDK provides typed, lower-level access to the control plane using
`client_credentials` (service account) authentication — for reads, ad-hoc
management, and anything not covered by the attribute sync above.

### Retries

`retryAttempts` (`FLOWCATALYST_RETRY_ATTEMPTS`, default 3) is the total number of attempts (1 turns
retries off), `retryDelay` (`FLOWCATALYST_RETRY_DELAY`, ms, default 100) the base of the backoff. A
request is sent again only when that is safe. It is **repeatable** when its method is GET, HEAD,
OPTIONS, PUT or DELETE, or when it is a POST the SDK marks: the declarative syncs (every resource's
`sync()`, `principals()->syncUsers()`, everything `flowcatalyst:sync` and `definitions()` send) and a
`dispatchJobs()->batch()` whose every job has an idempotency key. Every other POST (create, provision,
activate, suspend, archive, fire, reset-password, ...) is not.

| What happened | Repeatable | Not repeatable |
|---|---|---|
| 408, 429 (refused before it was processed) | retried | retried |
| connection never made (host not resolved, refused, connect timeout, TLS handshake) | retried | retried |
| 502, 503, 504 | retried | `OutcomeUnknownException`, not sent again |
| connection lost or timed out after the request was sent | retried | `OutcomeUnknownException`, not sent again |
| any other status | thrown | thrown |

`FlowCatalyst\Exceptions\OutcomeUnknownException` (a `FlowCatalystException`) means the platform may
have applied the request and its answer was lost. It is not a refusal: read the state before deciding
to send it again. It carries `$method`, `$endpoint` and `$status` (the gateway's, or null after a
transport failure, which is `getPrevious()`); `getCode()` is the status or 0, `getContext()` the
gateway's body.

```php
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\OutcomeUnknownException;

try {
    $provisioned = $client->admin()->applications()->provisionServiceAccount($appId);
} catch (OutcomeUnknownException $e) {
    try {
        $client->admin()->applications()->getServiceAccount($appId);
        // it was provisioned, and its one-time secrets are lost: rotate them on the service account
    } catch (FlowCatalystException $notFound) {
        // a 404: it was not, and provisioning again is safe
    }
}
```

A DELETE whose earlier attempt ended with an unknown outcome and whose retry answers 404 succeeds
(the first attempt most likely deleted it); a 404 on the first attempt is still thrown. For a custom
POST through `request()` that you know is safe to send twice, pass `repeatable: true`.

### Event Types

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\DTOs\Requests\AddSchemaVersionRequest;
use FlowCatalyst\DTOs\Requests\CreateEventTypeRequest;

// List event types
$result = FlowCatalyst::eventTypes()->list();
foreach ($result->items as $eventType) {
    echo $eventType->code . ': ' . $eventType->name;
}

// Create an event type (returns the new id)
$eventTypeId = FlowCatalyst::eventTypes()->create(new CreateEventTypeRequest(
    code: 'order:fulfillment:order:created',
    name: 'Order Created',
    description: 'Fired when a new order is placed',
));

// Add a schema version
FlowCatalyst::eventTypes()->addSchemaVersion($eventTypeId, new AddSchemaVersionRequest(
    schema: ['type' => 'object', 'properties' => ['orderId' => ['type' => 'string']]],
    version: '1.0',
));
```

### Admin facade (`admin()`)

User, client and application management and router checks live under `admin()`. They call the
same resource classes and behave identically; only the deprecation notice differs.

```php
use FlowCatalyst\Facades\FlowCatalyst;

FlowCatalyst::admin()->principals()->findByEmail($email);
FlowCatalyst::admin()->principals()->addRole($principalId, 'myapp:editor');
FlowCatalyst::admin()->clients()->create(new CreateClientRequest(identifier: 'acme', name: 'Acme'));
FlowCatalyst::admin()->applications()->listClients($applicationId);
FlowCatalyst::admin()->router()->inPipelineBatch($messageIds);
```

Calling the same admin methods through the old accessors (`principals()`, `clients()`,
`applications()`, `router()`) still works, but raises one `E_USER_DEPRECATED` per method per process.
Silence it with `error_reporting()` or an error handler. Not deprecated: every `sync*` method,
`clients()->list/get/getByIdentifier/getRaw/getByIdentifierRaw`, `applications()->get/getByCode/listRoles`,
`principals()->setClientAssociation`, outbox, dispatch-job creation, auth, `request()` and the
constructor options. See `CHANGES.md` ("Migrating to admin()") for the exact method list.

### Platform config, email-domain mappings, client association, raw lookups

```php
use FlowCatalyst\Facades\FlowCatalyst;

// Platform config (anchors, or roles with a platform-config access grant)
$row  = FlowCatalyst::platformConfig()->get('rfp', 'mail', 'from');          // ?clientId= via 4th arg
$row  = FlowCatalyst::platformConfig()->set('rfp', 'mail', 'from', 'a@b.c'); // PLAIN by default
FlowCatalyst::platformConfig()->delete('rfp', 'mail', 'from');              // idempotent
$rows = FlowCatalyst::platformConfig()->list('rfp');                         // listRaw() for the body as-is

// SECRET properties come back from get()/list() as the literal "***" for every caller.
// The only way to read the plaintext is the audited reveal call. It needs a reveal permission:
// platform:admin:config:reveal (a platform administrator), or platform:application-service:config:reveal
// (an application's service account, confined to its own applications and to the clients it is linked
// to: its explicit clients, else the clients its application is enabled for, plus the global value):
$secret = FlowCatalyst::platformConfig()->revealSecret('rfp', 'mail', 'smtp_password'); // ?clientId= via 4th arg
$secret->value;   // plaintext; var_dump/json_encode/toArray() show "***". Uncached, never logged.
// 403 not authorised, 404 no such property, 400 NOT_A_SECRET (PLAIN: use get()) -> FlowCatalystException

// Writing "***" or an empty value to a SECRET is refused (400 SECRET_VALUE_MASKED / SECRET_VALUE_REQUIRED,
// PlatformConfig::ERROR_SECRET_VALUE_MASKED / ERROR_SECRET_VALUE_REQUIRED); to keep a secret, do not set it.

// Acting for a signed-in user: set/delete/revealSecret take onBehalfOf, sent as X-On-Behalf-Of and recorded
// in the audit row (service accounts only; at most 256 printable characters, else 400 INVALID_ON_BEHALF_OF).
FlowCatalyst::platformConfig()->set('rfp', 'mail', 'from', 'a@b.c', onBehalfOf: $user->email);
$secret = FlowCatalyst::platformConfig()->revealSecret('rfp', 'mail', 'smtp_password', onBehalfOf: $user->email);
// List rows carry clientIdentifier (e.g. "acme") beside the stored clientId.

// Email-domain mapping by domain (404 FlowCatalystException when unmapped)
$mapping = FlowCatalyst::emailDomainMappings()->getByDomain('example.com');

// Change a USER's scope/client association (anchor-only). '*' = ANCHOR;
// a client id needs mode CHANGE_CLIENT or TO_PARTNER.
FlowCatalyst::admin()->principals()->setClientAssociation($principalId, $clientId, 'TO_PARTNER');

// The platform's own answer, with no DTO defaults (get() fills a missing status with ACTIVE)
$raw = FlowCatalyst::clients()->getRaw($clientId);
$raw = FlowCatalyst::clients()->getByIdentifierRaw('acme');
```

### Subscriptions

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\DTOs\EventTypeBinding;
use FlowCatalyst\DTOs\Requests\CreateSubscriptionRequest;
use FlowCatalyst\Enums\DispatchMode;

// Create a subscription (returns the new id)
$subscriptionId = FlowCatalyst::subscriptions()->create(new CreateSubscriptionRequest(
    code: 'notify-warehouse',
    name: 'Notify Warehouse',
    endpoint: 'https://warehouse.example.com/webhook',
    eventTypes: [new EventTypeBinding('order:fulfillment:order:created')],
    dispatchPoolId: $poolId,
    mode: DispatchMode::IMMEDIATE,
    timeoutSeconds: 30,
    maxRetries: 5,
));

// Pause/resume a subscription
FlowCatalyst::subscriptions()->pause($subscriptionId);
FlowCatalyst::subscriptions()->resume($subscriptionId);
```

### Dispatch Pools

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\DTOs\Requests\CreateDispatchPoolRequest;

// Create a dispatch pool for rate limiting (returns the new id)
$poolId = FlowCatalyst::dispatchPools()->create(new CreateDispatchPoolRequest(
    code: 'warehouse-webhooks',
    name: 'Warehouse Webhooks',
    rateLimit: 100,      // Max 100 requests per minute
    concurrency: 10,     // Max 10 concurrent requests
));

// Suspend/activate a pool
FlowCatalyst::dispatchPools()->suspend($poolId);
FlowCatalyst::dispatchPools()->activate($poolId);
```

### Roles & Permissions

To **declare** roles/permissions, prefer the attribute sync in
[Declaring & syncing roles and permissions](#declaring--syncing-roles-and-permissions).
For reads and ad-hoc lookups:

```php
use FlowCatalyst\Facades\FlowCatalyst;

$roles = FlowCatalyst::roles()->list();
$role = FlowCatalyst::roles()->get('myapp:admin');   // includes its permissions
$permissions = FlowCatalyst::permissions()->list();
```

### Applications

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\DTOs\Requests\CreateApplicationRequest;

// List applications
$result = FlowCatalyst::admin()->applications()->list();

// Get by code
$app = FlowCatalyst::applications()->getByCode('myapp');

// Create an application
$app = FlowCatalyst::admin()->applications()->create(new CreateApplicationRequest(
    code: 'myapp',
    name: 'My Application',
    description: 'My awesome application',
));
```

### Ingestions

```php
use FlowCatalyst\DTOs\Ingestion\{IngestionDefinition, IngestionSource, IngestionSplit, IngestionTarget};

$definition = new IngestionDefinition(
    name: 'Orders drop',
    source: new IngestionSource(IngestionSource::SFTP, [/* host, directory, credential: ['name' => 'sftp-password'], ... */]),
    split: IngestionSplit::csv(header: true),
    target: new IngestionTarget('order', 'https://app.example.com/hook', pool: 'default', mode: IngestionTarget::MODE_NEXT_ON_ERROR, batchSize: 10),
);

$ing = FlowCatalyst::ingestions()->create('edi', 'acme', 'orders', $definition);
FlowCatalyst::ingestions()->update($ing->id, $definition, expectedVersion: $ing->version);

$ing->save?->outcome;        // SAVED, MADE_LIVE, DRAFT, DRAFT_EXISTS or UNCHANGED
$ing->save?->awaitingPromote; // true for a draft someone with promote must make live

// Notices: failures the platform refused (category "rejected"); $ing->notices counts the open ones
$open = FlowCatalyst::ingestions()->failures($ing->id, category: 'rejected', undismissed: true);
FlowCatalyst::ingestions()->dismissFailure($ing->id, $open['failures'][0]['id']); // stays listed

// The secret names the live definition binds, with each runtime's last fetch (never a value)
$secrets = FlowCatalyst::ingestions()->secrets($ing->id);
```

An ingestion has an INTEGRATION application (code) and a client (id or identifier). Secrets are referenced by
name, never passed as values. A runtime-supplied source kind is `IngestionSource::custom($type, $settings)`.
`ingestionSets()->apply($name, $items, dryRun: true)` plans a set apply without writing.

Without the promote permission a create or update saves the definition as a **draft** (`save->outcome` is
`DRAFT`), never a refusal; someone who holds promote makes it live with `makeVersionLive()`. A definition an
existing version already says is not saved again; versions carry `draft` and `digest`. `$ing->warnings` names
what will not run as meant, for example an sftp source with neither `knownHosts` nor `acceptAnyHostKey`
(`acceptAnyHostKey: true` connects without checking the host key and needs the bind-secrets permission).

## Portal identities

Portal users (`ptu_…`) are a separate end-user population from platform
users — a client's customers, invited per client, with no platform SSO
reuse. For the login side, set `FLOWCATALYST_OIDC_PORTAL=true` so the OIDC
flow enters through `/portal/authorize`. For managing portal users, the
full admin surface lives on the generated client:

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\Generated\Model\PortalUserRequest;

$api = FlowCatalyst::generated();

// Invite (idempotent): returns identityId, created/invited flags, inviteUrl
$result = $api->ensurePortalUser((new PortalUserRequest())
    ->setClientId($clientId)
    ->setEmail('pat@customer.example')
    ->setReturnInviteLink(true));

$users = $api->listPortalUsers(['clientId' => $clientId]);
```

`activatePortalUser` / `deactivatePortalUser` / `deletePortalUser` cover the
rest of the lifecycle. All credential UX (login page, invites, self-service
forgot-password) is platform-hosted — the portal app never implements any
of it. Full walkthrough:
[docs/portal-implementation-guide.md](../../docs/portal-implementation-guide.md).

## Field workers

Field workers (drivers, pickers, yard staff) sign in on an **enrolled device** with a PIN, at the
platform's `/field/*`. The platform issues a short RS256 access token (`typ: fc-field+jwt`,
`token_use: "field"`, `aud` = your application code) bound to the device key (`cnf.jkt`). The device
presents it to your backend as `Authorization: DPoP <token>` plus a `DPoP: <proof>` header (an ES256
JWS signed by the device key). PINs never reach your backend: the SDK only verifies.

```env
FLOWCATALYST_FIELD_APP_CODE=my-app
# Public origin devices call (scheme://host[:port]); defaults to APP_URL. The proof's htu is
# checked against this + the request path. With one origin the Host header is never used.
# Several (comma-separated) or a wildcard: see "An application with a host per client".
FLOWCATALYST_FIELD_APP_BASE_URL=https://app.example.com
```

The issuer and signing keys come from `FLOWCATALYST_BASE_URL` (OIDC discovery + JWKS, shared with the
other validators). Optional: `FLOWCATALYST_FIELD_PROOF_MAX_AGE` (seconds a proof's `iat` may differ from
now, default 300; widen for devices with bad clocks) and `FLOWCATALYST_FIELD_CLOCK_SKEW` (default 60).

```php
// Routes: the `fc.field` middleware authenticates with the `field` guard.
Route::post('/api/deliveries', function (Request $request) {
    /** @var \FlowCatalyst\Auth\Field\FieldPrincipal $worker */
    $worker = $request->user();   // or Auth::guard('field')->user()
    $worker->workerId; $worker->workerCode; $worker->clientIdentifier; $worker->deviceId;
})->middleware('fc.field');
```

- A verified request carries a `FieldPrincipal` (`workerId`, `clientId`, `clientIdentifier`,
  `workerCode`, `name`, `appCode`, `deviceId`, `deviceMode`, `sessionId`, `amr`, `authTime`,
  `expiresAt`, `jkt`, `tokenId`). It is also the `Authenticatable` of the `field` guard (identifier =
  worker id), so `auth:field` works, but the middleware gives proper error responses.
- A token that may only change the PIN (`pcr`) answers `403 {"error":"PIN_CHANGE_REQUIRED"}`.
  Every other refusal answers `401 {"error": <reason>}` with `WWW-Authenticate: DPoP error="invalid_token"`;
  reasons are `malformed`, `wrong_typ`, `wrong_token_use`, `wrong_aud`, `expired`, `invalid_token`,
  `missing_proof`, `invalid_proof`, `jkt_mismatch`, `htm_mismatch`, `htu_mismatch`, `ath_mismatch`,
  `stale_proof`. A JWKS fetch failure answers `503`. If you bind a `FieldClientCheck` (below) and it
  refuses a worker, the answer is `403 {"error":"WRONG_CLIENT"}` with no `WWW-Authenticate` (reason `wrong_client`).
- Access tokens must be RS256 and proofs ES256; any other `alg` is refused. Replay protection beyond the
  proof's `iat` window (a nonce) is not done by the SDK.
- To use the verifier directly: `app(\FlowCatalyst\Auth\Field\FieldTokenVerifier::class)->verify($authorizationHeader, $dpopHeader, $method, $fullUrl)`
  returns a `FieldPrincipal` or throws `FieldTokenException` (`->reason`).
- Platform API access-token validation (`AccessTokenValidator`, the `fc-token` guard) refuses any
  `token_use` other than `api` or `identity` (or absent), so a field token is never an API credential.

Online check (the session must still be live; needs `platform:iam:field-token:introspect`):

```php
$r = FlowCatalyst::introspectFieldToken($token);   // ->getActive(), ->getSub(), ->getWorkerCode() ...
```

Admin wrappers over the generated client (permissions `platform:iam:field-worker:*`,
`platform:iam:field-device:*`):

```php
$fc = FlowCatalyst::fieldWorkers();
// No worker code: the platform generates one (e.g. KTM482) and returns it; the idempotency key
// (your own record's id) lets a repeat answer the same worker instead of making a second one.
$w = $fc->ensure((new EnsureFieldWorkerRequest())->setClientId($cid)->setName('Ann')->setAppCodes(['my-app'])
    ->setIdempotencyKey('employee-1042'));
$w->getWorkerCode();             // what the worker types; or ->setWorkerCode('W001') for a code of your own
$fc->list($cid, ['appCode' => 'my-app']); $fc->get($id); $fc->update($id, $req); $fc->delete($id);
$fc->resetPin($id);              // random temporary PIN, returned once; resetPin($id, '482913') to set one
$fc->unlock($id); $fc->suspend($id); $fc->activate($id);
$fc->grantApp($id, 'my-app'); $fc->revokeApp($id, 'my-app'); $fc->signIns($id, page: 0);

$dev = FlowCatalyst::fieldDevices();
$dev->createEnrolmentCode((new CreateEnrolmentCodeRequest())->setClientId($cid)->setMode('PERSONAL')->setAppCodes(['my-app']));
$dev->list($cid); $dev->get($id); $dev->suspend($id); $dev->activate($id); $dev->revoke($id);

FlowCatalyst::fieldPolicies()->get($cid);   // ->update($cid, fn ($r) => $r->setWorkerCodeDigits(3..6)); put() replaces the whole policy
```

**Photos.** A worker can have a photograph (shown to the people who manage and work with them):

```php
// The bytes of a JPEG, PNG or WebP. The platform decides the type from the bytes, scales it to
// 512 px (full) and 128 px (thumb) and keeps nothing else of the upload, metadata included.
// (The generated client sends it as application/octet-stream, which the platform accepts for all three.)
$w = $fc->setPhoto($id, file_get_contents($path));        // the refreshed worker; $w->getPhoto()?->getVersion()
$img = $fc->getPhoto($id);                                 // 'full' for the big one
$img->bytes; $img->contentType; $img->etag;                // FlowCatalyst\DTOs\FieldWorkerPhotoImage
$again = $fc->getPhoto($id, 'thumb', $img->etag);          // ->notModified is true (and ->bytes null) while unchanged
$fc->removePhoto($id);                                     // not an error when there is none
FlowCatalyst::fieldPolicies()->deletePhotos($cid);         // ->getRemoved(); every photo of the client's workers
```

The generated client cannot read an image answer (it declares none), so `getPhoto` reads the response
itself; a worker with no photo (or a client with photos off) throws a `FlowCatalystException` with code
404 and `getContext()['code']` `PHOTO_NOT_FOUND`. A refusal of `setPhoto` comes back as the generated
`ErrorModel` whose `error` is one of `PHOTO_EMPTY`, `PHOTO_TOO_LARGE` (413), `PHOTO_UNSUPPORTED_TYPE` (415),
`PHOTO_TYPE_MISMATCH`, `PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY` (503),
`PHOTOS_DISABLED_FOR_CLIENT` (409) or `PHOTOS_DISABLED` (404). The policy has `photosEnabled` (default true:
false hides the photos and keeps them) and `workerMaySetOwnPhoto` (default false: a signed-in worker may set their
own from the device); **a `put` of the policy replaces it, so a field left out takes its default** (read it first).
`deletePhotos` cannot be undone and is refused for an application-confined service account (403
`APPLICATION_FORBIDDEN`): it removes its own workers' photos one at a time.

**Unlock keys.** On a personal, enrolled device a worker can register an unlock key (held in the phone's secure
hardware, usable only after a fingerprint or face match) and sign in again with it instead of the PIN. The platform
shows every key, limits them by policy and revokes them:

```php
$keys = $fc->listUnlockKeys($id);                 // ->getUnlockKeys(): newest first, live and revoked
$key = $keys->getUnlockKeys()[0];                 // getStatus() ACTIVE|REVOKED, getLiveSessions(), getAttestation()->getVerdict(), ...
$key = $fc->revokeUnlockKey($id, $key->getId());  // a live key is revoked (ADMIN) and its sessions end; an already revoked
                                                  // key keeps its reason and has the sessions it left ended (getLiveSessions(), after)
FlowCatalyst::fieldDevices()->get($deviceId)?->getUnlockKey();   // the device's live key, or null
```

The policy has `unlockKeysEnabled` (default false; switching it off revokes every unlock key of the client),
`unlockKeyAttestation` (`NONE`, the default, records the key's attestation only; `REQUIRED` admits only a key with a
trusted Android key attestation) and `pinReauthDays` (0 to 90, now default **0**: no periodic PIN; a stored policy
keeps its value). **A `put` that does not set `unlockKeysEnabled` switches unlock keys off for the client, revokes all
its keys and signs those workers out**: change a policy with `fieldPolicies()->update($cid, fn ($r) => ...)`, which
reads it and writes every other setting back, and call `put` with a hand-made request only to set every setting.

**Session limits, and devices with no signal.** A field session has an idle limit (counted on the platform from the
device's last refresh) and an absolute limit (from the sign-in). The defaults: a shared device 15 minutes idle and
12 hours absolute (the 12 hours are fixed); a personal device 2 hours idle and 14 hours absolute; a device that is
not enrolled 15 minutes and 12 hours. The idle limits and the personal absolute limit are the client's field policy:

```php
FlowCatalyst::fieldPolicies()->update($cid, fn (PutFieldPolicyRequest $r) => $r
    ->setSharedIdleSecs(43200)                              // 60 to 86400, as the other idle limits
);                                                          // reads the policy, changes this, writes the rest back
```

A put replaces the whole policy: a value left out goes back to its default, so `update` reads the policy first and
writes every value you did not change back (`FieldPolicies::toRequest($policy)` is that request, to change by hand). The policy belongs to the client, not to your application: it applies to every application's
sessions for that client, and a caller confined to one application may not change it.

A device does not end a session by its own clock, so a worker with no signal stays signed in and the app goes on
recording into its offline queue. What your backend sees is this: when the signal returns the device refreshes; if a
limit passed meanwhile the platform ends the session, the worker signs in again, and the queued requests then arrive
under the **new** session, each signed when it is sent. So accept work whose own timestamp is older than the token's
`authTime`, and identify a queued request by an id the app gives it (an idempotency key), not by the session. The
device side is described in `@flowcatalyst/field-react-native`'s README, "No signal: what happens to a session".

### An application with a host per client

If each client reaches your application on its own host (`acme.example.com`, `globex.example.com`), one
fixed origin cannot be right for all of them. Give `app_base_url` a list, or a wildcard:

```php
// config/flowcatalyst.php
'field' => [
    'app_base_url' => 'https://*.example.com',
    // or a list: ['https://acme.example.com', 'https://globex.example.org:8443']
    // or, in the environment variable: https://acme.example.com,https://globex.example.org
],
```

- `*` must be the whole leftmost label and matches exactly one: `acme.example.com` matches,
  `a.b.example.com` and `example.com` do not. `https://*.com` and `https://*` are refused (a wildcard is
  for a zone, not a top-level name), as is a `*` anywhere else in the host. A bad entry throws a
  `LogicException`.
- With one origin and no wildcard nothing changes: the request's host is never read. Otherwise the guard
  reads the request's host, finds the first entry it matches, and expects the proof's `htu` to be that
  entry's scheme and port, the request's host, and the request path. The request's own scheme and port
  are not used (behind a load balancer they are not what the device called). A host that matches no
  entry is refused as `htu_mismatch`.
- Why this is safe: the host is untrusted, but it only chooses among origins your configuration allows.
  Someone who gets a device to call a host of theirs holds a proof for that host, which matches no
  entry. The one condition: **every wildcard must cover a DNS zone you wholly control.** If someone else
  can create `evil.example.com`, `https://*.example.com` is too wide. Behind a proxy, set Laravel's
  trusted proxies, so the host Laravel reads is the one the device called.

The SDK checks that the token is for this application (`aud`). It does not check which client the request
is for. An application with several origins or a wildcard almost certainly serves several clients, and
without a check a worker of one client is not stopped from calling another client's host, so the SDK
refuses to run that way: **with several origins or a wildcard you must bind a `FieldClientCheck`**, or the
`field` guard throws a `LogicException` on the first field request, before the token is read (the
`fc.field` middleware lets it through as a 500). An invalid `app_base_url` is reported first. If you have
one client on several hosts, bind a check that returns true. Bind one to decide it:

```php
use FlowCatalyst\Auth\Contracts\FieldClientCheck;
use FlowCatalyst\Auth\Field\FieldPrincipal;
use Illuminate\Http\Request;

final class TenantFieldClientCheck implements FieldClientCheck
{
    public function allows(FieldPrincipal $worker, Request $request): bool
    {
        // acme.example.com -> "acme"; the worker's client identifier comes from the token.
        $tenant = explode('.', $request->getHost())[0];
        return $worker->clientIdentifier === $tenant;
    }
}

// AppServiceProvider::register()
$this->app->bind(FieldClientCheck::class, TenantFieldClientCheck::class);
```

The SDK binds none. With one origin and no wildcard none is needed and nothing changes. An unknown host
is still `htu_mismatch`. The `field` guard calls
yours once per request, after the token and proof verified, and only for a token that is not a
PIN-change-only one (that still answers `PIN_CHANGE_REQUIRED`). When it returns `false`,
`Auth::guard('field')->user()` is null, `refusal()->reason` is `wrong_client`, and `fc.field` answers
`403 {"error":"WRONG_CLIENT"}` with no `WWW-Authenticate` header: the device libraries read `401` with
`DPoP error="invalid_token"` as "refresh, retry, then end the session", and this worker's session is good.
If your check throws, the exception is reported and the answer is `503 {"error":"unavailable"}`.
`FieldTokenVerifier::verify()` called directly does not run the check; it belongs to the guard and
middleware.

## Outbox (Event Creation)

The outbox allows your application to create events and dispatch jobs using the outbox pattern. Events are written to a local database table and then forwarded to FlowCatalyst by a poller (`fcdev outbox` locally, or the standalone outbox poller in production).

Event `type`s and dispatch-job `code`s must be fully qualified
`application:subdomain:aggregate:action` strings; the SDK rejects bare codes.

### Setup

Publish and run the migration:

```bash
php artisan vendor:publish --tag=flowcatalyst-migrations
php artisan migrate
```

### Creating Events

```php
use FlowCatalyst\Facades\Outbox;
use FlowCatalyst\Outbox\DTOs\CreateEventDto;

// Create a single event
$eventId = Outbox::createEvent(
    CreateEventDto::create(
        type: 'shop:orders:order:created',
        data: ['orderId' => 'ORD-123', 'total' => 99.99, 'currency' => 'USD'],
    )->withCorrelationId('corr-abc-123')
      ->withSource('order-service')
      ->withMessageGroup('orders')   // per-group FIFO ordering
);

// Batch create events
$eventIds = Outbox::createEvents([
    CreateEventDto::create('shop:orders:order:created', ['orderId' => 'ORD-001']),
    CreateEventDto::create('shop:orders:order:created', ['orderId' => 'ORD-002']),
    CreateEventDto::create('shop:orders:order:created', ['orderId' => 'ORD-003']),
]);
```

### Client-centric linkage

FlowCatalyst is client-centric. Name the application and client your emitted
events / audit logs belong to once, and the platform resolves them at ingest:

```env
FLOWCATALYST_APP_CODE=blog              # application (also the event-type prefix)
FLOWCATALYST_CLIENT_CODE=acme           # client (by code → client_id at ingest)
```

Events emitted through the use-case envelope carry the **client** code (the
application is derived from the event-type prefix); **audit logs** carry both.
An unknown / unset client code simply leaves the link empty — the event still
ingests (an event is a fact; the platform keeps it).

### Branded IDs

Mint platform-style typed ids for your own entities (matching `aud_…`, `prn_…`):

```php
use FlowCatalyst\UseCase\DomainEventHelpers;

DomainEventHelpers::generateId();        // raw 13-char TSID
DomainEventHelpers::generateId('cmt');   // branded → "cmt_6F7JC2A6JFR7N"
```

### Creating Dispatch Jobs

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\Facades\Outbox;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;

// Create a dispatch job (direct webhook without subscription matching)
$job = CreateDispatchJobDto::create(
        source: 'order-service',
        code: 'shop:orders:warehouse:notify',
        targetUrl: 'https://warehouse.example.com/webhook',
        payload: ['orderId' => 'ORD-123', 'action' => 'prepare'],
        dispatchPoolId: $warehousePoolId,
    )->withCorrelationId('corr-abc-123')
    ->withClientCode('acme')                  // the job's client: deliveries carry X-FlowCatalyst-Client
    ->withConnectionId($connectionId)         // its service account signs the delivery
    ->withIdempotencyKey('warehouse-ORD-123') // a repeat answers the first job's id
    ->withMetadata(['orderId' => 'ORD-123']); // sent as [{key, value}]

// Through the outbox (returns the outbox row id)...
$outboxId = Outbox::createDispatchJob($job);

// ...or straight to the platform (POST /api/dispatch-jobs/batch; returns the job id)
$jobId = FlowCatalyst::dispatchJobs()->create($job);
$results = FlowCatalyst::dispatchJobs()->batch([$job, $other]); // list<DispatchJobResult>: id, status, error
```

Both paths send the same item (`CreateDispatchJobDto::toBatchItem()`).

- `withConnectionId()` names the connection the job is delivered through: pausing the connection holds
  the job, and (for a job with no subscription) the connection's service account signs the delivery.
  It must be a global connection or the job's client's, and shared or the job's application's.
- `withIdempotencyKey()` (at most 100 characters) makes the create idempotent within the job's
  application (the first segment of its code) and client: a repeat creates nothing and answers the
  first job's id. Set one on every job of a direct call: only a batch whose every job has a key is sent
  again after a 502-504 or a connection lost after sending (see [Retries](#retries)).
- `withClientCode()` / `withClientId()` set the job's FlowCatalyst client (an id wins over a code).
- `withMetadata()` takes an associative array (sent key-sorted as `[{key, value}]`, values as strings)
  or a list of `{key, value}` pairs.
- `withHeaders()`, `scheduledFor()`, `expiresAt()` and the `retryStrategy` argument are **not sent**:
  the platform's batch route does not honour them. They remain only so existing code compiles.
- `dispatchJobs()` needs `platform:messaging:batch:dispatch-jobs-write`. The batch is all-or-nothing:
  a refused item fails the request with a `FlowCatalystException` and creates nothing.

## Push to clients

Tell the people looking at a screen that something changed, without holding a
stream yourself. The app writes a `PUSH` row to its outbox **in the same
transaction as the change**; the platform's outbox processor sends it to the
push gateway, which fans it out over server-sent events to the clients listening
on the topic. Pushes are signals (the client reads the change again): durable
work stays events and dispatch jobs. The wire and the readers (field-js,
field-react-native `PushStream`, base URL `{platform}/api`) are unchanged.

> The platform's outbox processor must already know the `PUSH` type (fcdev and
> the processor image that ship the push gateway do). An older processor posts
> an unknown type to the bare platform URL and fails the row.

### Write a push

```php
use FlowCatalyst\Facades\Outbox;
use FlowCatalyst\Outbox\DTOs\CreatePushDto;

DB::transaction(function () use ($order) {
    $order->save();

    Outbox::createPush(
        CreatePushDto::create("shop:orders:{$order->id}", 'order.updated', ['status' => 'shipped']),
    );
});
```

- `topic` is `{application}:{segment}(:{segment})*`, the first segment the
  application (your credential must be allowed to push for it); `type` is
  `[a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*` (`reset`, `auth-expired`, `subscribed`
  and `fw.*` are reserved); `data` is a JSON object (an associative array, an
  object, or JSON text holding one; `[]` is `{}`), 16 KiB by default, up to
  64 KiB with `->withMaxDataBytes($n)`.
- A push the platform would refuse is refused when it is built: `createPush`
  throws a `PushException` whose `errorCode()` is `TOPIC_INVALID`,
  `TYPE_INVALID`, `TYPE_RESERVED`, `DATA_INVALID`, `DATA_NOT_OBJECT`,
  `DATA_TOO_LARGE`, `PUSH_ID_INVALID` or `CLIENT_INVALID`.
- The default client of a push is `flowcatalyst.outbox.client_code`
  (`FLOWCATALYST_CLIENT_CODE`; `_platform` leaves it out); override per push
  with `->withClientCode('acme')`.
- **Ordering.** The row's message group is the topic, so the pushes of one topic
  reach the gateway in order (one HTTP call per push). A signal-only topic that
  does not need order can opt out: `->withUngrouped()` (batched, concurrent, no
  order), or name another group with `->withMessageGroup($g)`.
- The row id is the `pushId` (a TSID unless you `->withId(...)`, at most 26
  characters); the platform writes a `pushId` once.
- **Instant delivery.** With the `database` driver on **PostgreSQL** the insert
  also runs `pg_notify('fc_outbox_push', '')` in the same statement, so the
  processor wakes when your transaction commits (a rollback notifies nobody),
  typically within a few milliseconds. **MySQL and MongoDB cannot notify**: the
  row is the plain insert and the processor's poll delivers it (default 1 s).

### In a use case

```php
return Plan::save($order, $repo, $event)->andPush(
    CreatePushDto::create("shop:orders:{$order->id()}", 'order.updated', ['status' => 'placed']),
    fn (OrderPlaced $e) => CreatePushDto::create('shop:board', 'refresh', ['eventId' => $e->eventId()])->withUngrouped(),
);
```

The pushes are written after the event, in the plan's transaction. An invalid
push fails the plan with a validation error (its code is the push's) and the
transaction rolls back. The unit of work must implement `PushesUnitOfWork`
(`OutboxUnitOfWork`, the default, does); its `clientCode` is the default client.

### Who may listen: rules, keys, tickets

A client listens to a topic when a **rule** allows it or it holds a **ticket**.
Declare them in `config/flowcatalyst.php`; `php artisan flowcatalyst:sync` sends
them to the gateway (`--push` syncs only these). It is a full replace of both,
and happens only when at least one is configured:

```php
'push' => [
    'rules' => [
        ['pattern' => 'shop:orders:{id}', 'requires' => ['shop:orders:order:view']],
        ['pattern' => 'shop:user:{principal}'],              // only your own
        ['pattern' => 'shop:announcements', 'public' => true], // deliberately open
    ],
    // "kid:base64url-seed": Ed25519, 32-byte seed (a secret)
    'ticket_key' => env('FLOWCATALYST_PUSH_TICKET_KEY'),
    'previous_ticket_keys' => env('FLOWCATALYST_PUSH_PREVIOUS_TICKET_KEYS'), // comma-separated, during a rotation
],
```

Or in code: `SyncDefinitionSet::forApplication('shop')->withPush($rules, $minter)`
(a `TicketMinter`, or a JWKS array), also from a `ProvidesSyncDefinitionSets`
provider. `SyncOptions::pushOnly()` / `syncPush: false` select or skip the step;
the result's `push` has the gateway's counts (`rules`, `keys`) and what it holds
(`syncedPatterns`, `syncedKids`).

The first segment of every pattern is your application code. `{principal}` must
equal the subscriber's principal id, `{client}` the stream's client, any other
`{name}` matches one segment. A rule with neither `{principal}` nor `requires`
must say `public: true`. The gateway refuses (400, with a code) rules for
another application's patterns, duplicates, more than 200 rules, more than 4
keys, or keys that are not public Ed25519 JWKs.

**Rotation:** add the new key (keep the old one in `previous_ticket_keys`), sync,
start signing with the new key, wait one ticket lifetime (at most 5 minutes),
drop the old key, sync again.

### Mint a ticket

For what a rule cannot express (this user may see order 7 but not order 8), mint
a ticket in your own endpoint, after your own access check:

```php
use FlowCatalyst\Push\TicketMinter;

$minter = TicketMinter::fromConfig(
    config('flowcatalyst.application_code'),
    config('flowcatalyst.push.ticket_key'),
);

$ticket = $minter->mint(
    client: 'acme',                                   // the stream's client ('_platform' for platform-wide)
    topics: ['shop:orders:ord_7', 'shop:orders:ord_8:{part}'], // placeholders match one segment
    sub: 'prn_1',                                     // optional: recorded in logs
    // jkt: '<dpop key thumbprint>',                  // optional: the subscribe must prove that key
    // ttlSeconds: 60,                                // 1 to 300, default 60: the time to subscribe with it
    // streamExp: new DateTimeImmutable('+1 hour'),   // optional: the latest the stream may live
);
// hand $ticket to the browser; it POSTs {topics, tickets: [$ticket]} to {platform}/api/push/subscriptions
```

A ticket is an EdDSA JWS (`iss` = your application code, `aud` `fw-push`, header
`kid`), signed with ext-sodium (`sodium_crypto_sign_detached`). `mint` throws a
`PushTicketException` for a topic outside your application, more than 100
topics and a lifetime over 5 minutes, because the gateway would grant nothing.
`$minter->jwks()` is the JWKS that `GET /push/keys` serves and sync sends.

## Webhook Validation

Validate incoming webhooks from FlowCatalyst using HMAC-SHA256 signatures.

### Using Middleware

```php
// routes/api.php
Route::post('/webhooks/flowcatalyst', [WebhookController::class, 'handle'])
    ->middleware('flowcatalyst.webhook');
```

### Manual Validation

```php
use FlowCatalyst\Webhook\WebhookValidator;
use FlowCatalyst\Exceptions\WebhookValidationException;

public function handleWebhook(Request $request)
{
    try {
        $validator = WebhookValidator::fromConfig();
        $validator->validateRequest($request);
    } catch (WebhookValidationException $e) {
        return response()->json(['error' => 'Invalid signature'], 401);
    }

    // Process the webhook
    $payload = $request->json()->all();

    return response()->json(['received' => true]);
}
```

## Database Requirements

### MySQL 8.0+

MySQL 8.0 or higher is required for native JSON column support.

```bash
composer require doctrine/dbal
php artisan vendor:publish --tag=flowcatalyst-migrations
php artisan migrate
```

### PostgreSQL 12+

PostgreSQL 12+ is fully supported with JSONB columns.

```bash
composer require doctrine/dbal
php artisan vendor:publish --tag=flowcatalyst-migrations
php artisan migrate
```

### MongoDB 4.4+

For MongoDB, install the Laravel MongoDB package:

```bash
composer require mongodb/laravel-mongodb
```

Add to `config/database.php`:

```php
'mongodb' => [
    'driver' => 'mongodb',
    'host' => env('MONGO_HOST', 'localhost'),
    'port' => env('MONGO_PORT', 27017),
    'database' => env('MONGO_DATABASE'),
    'username' => env('MONGO_USERNAME'),
    'password' => env('MONGO_PASSWORD'),
],
```

Configure in `.env`:

```env
FLOWCATALYST_OUTBOX_DRIVER=mongodb
FLOWCATALYST_OUTBOX_CONNECTION=mongodb
```

Create the collection with indexes:

```javascript
db.createCollection('outbox_messages');
db.outbox_messages.createIndex(
  { clientId: 1, status: 1, createdAt: 1 },
  { name: 'idx_outbox_pending' },
);
db.outbox_messages.createIndex({ status: 1, createdAt: 1 }, { name: 'idx_outbox_status' });
```

## Error Handling

The SDK throws specific exceptions for different error types:

```php
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\AuthenticationException;
use FlowCatalyst\Exceptions\ValidationException;
use FlowCatalyst\Exceptions\OutboxException;

try {
    FlowCatalyst::eventTypes()->create(new CreateEventTypeRequest(...));
} catch (AuthenticationException $e) {
    // Invalid credentials or token expired
} catch (ValidationException $e) {
    // Validation errors
    $errors = $e->getErrors();
} catch (FlowCatalystException $e) {
    // General API error
    $context = $e->getContext();
}
```

## AI Agent Access (MCP Server)

If you're using an AI coding agent (Claude Code, Cursor, Windsurf, etc.), you can give it read-only access to your FlowCatalyst event types, schemas, and subscriptions via the FlowCatalyst MCP server. This lets the agent explore your event catalog and generate typed code (including PHP DTOs) for you.

The MCP server is built into the platform binaries; there is no separate npm package. Two ways to run it:

- **`fcdev mcp`**: stdio by default (the usual way an MCP client launches a server as a subprocess); pass `--http <bind>` (for example `--http 127.0.0.1:8090`) to serve streamable HTTP instead. Flags `--platform-url`, `--client-id` and `--client-secret` override the `FLOWCATALYST_URL`, `FLOWCATALYST_CLIENT_ID` and `FLOWCATALYST_CLIENT_SECRET` environment variables. `fcdev start --mcp` boots it alongside the dev environment.
- **`fc-server`** hosts it when `FC_MCP_ENABLED` is set.

### Quick setup (Claude Code)

```bash
claude mcp add flowcatalyst -- fcdev mcp
```

### Quick setup (Cursor / Windsurf / Claude Desktop)

Add to your MCP config file (`.cursor/mcp.json`, Claude Desktop config, etc.):

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "fcdev",
      "args": ["mcp"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

You need a service account with the `AI Agent Read-Only` role. The agent can then generate PHP DTOs, TypeScript interfaces, Python dataclasses, or Java records from your event schemas.

## Testing

For testing, you can mock the facades:

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\Facades\Outbox;

FlowCatalyst::shouldReceive('eventTypes->list')
    ->andReturn(new EventTypeList(items: [], total: 0));

Outbox::shouldReceive('createEvent')
    ->andReturn('0HZXEQ5Y8JY5Z');
```

## License

MIT License. See [LICENSE](LICENSE) for details.
