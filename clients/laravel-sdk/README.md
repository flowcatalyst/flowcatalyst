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
- **Postbox** — create events/dispatch jobs via the transactional outbox.
- **Control plane client** — typed access to event types, subscriptions, dispatch
  pools, applications, roles, and more.
- **Webhook validation** — verify inbound HMAC-signed deliveries.

## Requirements

- PHP 8.1+
- Laravel 10, 11, 12, or 13
- For the Postbox: MySQL 8.0+ / PostgreSQL 12+ / MongoDB 4.4+
- For local authorization mirroring (optional): `spatie/laravel-permission`

## Contents

- [Installation](#installation) · [Local dev with `fc-dev`](#local-development-with-fc-dev)
- [Authentication & authorization](#authentication--authorization) — login, guards, permissions, modes, refresh
- [Declaring & syncing roles and permissions](#declaring--syncing-roles-and-permissions)
- [Control plane API](#control-plane-api) · [Postbox](#postbox-event-creation) · [Webhooks](#webhook-validation)

## Installation

```bash
composer require flowcatalyst/laravel-sdk
```

Publish the configuration file:

```bash
php artisan vendor:publish --tag=flowcatalyst-config
```

## Local development with `fc-dev`

For local work you need a FlowCatalyst control plane to talk to.
`fc-dev` is the official one-binary dev environment — bundled
PostgreSQL, platform API, message router, scheduler, and frontend
in a single process.

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex

fc-dev          # starts API on http://localhost:8080
```

If you use the **Postbox** (outbox pattern), you also need
`fc-dev outbox` running as a sidecar — it polls your app's
`outbox_messages` table and forwards events to the platform.
This sits alongside the `php artisan flowcatalyst:postbox:dispatch`
queue worker (or replaces it for setups where you'd rather not run
Laravel's queue runner for outbox forwarding).

```bash
# In your Laravel project directory:

# Once: write FC_OUTBOX_DB_URL / FC_OUTBOX_API_URL / FC_OUTBOX_TOKEN
# into ./.env (0600 perms; no secrets on argv or shell history).
# fc-dev outbox init appends to your existing .env — your other
# FLOWCATALYST_* keys are not touched.
fc-dev outbox init

# Daily: reads .env, auto-creates the `outbox_messages` table on
# first run if your Postbox migration hasn't been run yet, then polls.
fc-dev outbox poll
```

Complete reference: [fc-dev CLI docs](https://github.com/flowcatalyst/flowcatalyst-rust/blob/main/docs/developers/fc-dev.md).

## Configuration

Add the following to your `.env` file:

```env
# FlowCatalyst API
FLOWCATALYST_BASE_URL=https://your-instance.flowcatalyst.io
FLOWCATALYST_CLIENT_ID=your_client_id
FLOWCATALYST_CLIENT_SECRET=your_client_secret

# Postbox (for event creation)
FLOWCATALYST_TENANT_ID=your_tenant_id
FLOWCATALYST_POSTBOX_DRIVER=database

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

That's it. A guest hitting an `auth` route is **automatically redirected into the
FlowCatalyst login flow** — no `login` route or custom redirect needed. (If your
app defines its own `login` route, that one wins; the SDK won't hijack a local
login page. Opt out entirely with `FLOWCATALYST_OIDC_AUTO_GUEST_REDIRECT=false`.)

The routes `/flowcatalyst/{login,callback,logout,refresh}` are registered for
you. On your FlowCatalyst OAuth client, set the grant to `authorization_code`
and the redirect URI to **`https://your-app.com/flowcatalyst/callback`** (it must
match exactly — host, port, and scheme — including `localhost` vs `127.0.0.1`).

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

Both resolve a `FlowCatalystAuthenticatable` — read-only, no DB. Set
`FLOWCATALYST_OIDC_PERMISSION_RESOLVER=api_me` to resolve via `/api/me` instead
(one cached HTTP call, fresher on changes), or bind an offline `RbacCatalogue`.

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
| `FLOWCATALYST_OIDC_CLIENT_ID` / `_SECRET` | – | your OAuth login client (secret optional for PKCE) |
| `FLOWCATALYST_OIDC_SCOPE` | `openid profile email` | add `offline_access` for refresh tokens |
| `FLOWCATALYST_OIDC_HANDLER` | `database` | `database` (users table) or `session` (stateless) |
| `FLOWCATALYST_OIDC_PERMISSION_RESOLVER` | `token` | `token` (scope claim) or `api_me` (`/api/me`) |
| `FLOWCATALYST_OIDC_AUTO_GUEST_REDIRECT` | `true` | redirect stock-`auth` guests into OIDC |
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
covers `#[AsEventType]`, `#[AsSubscription]`, `#[AsDispatchPool]`, `#[AsProcess]`,
and `#[AsScheduledJob]`. See [`docs/syncing-definitions.md`](docs/syncing-definitions.md)
for the full reference, including the programmatic `DefinitionSynchronizer` for
multi-application setups.

## Control Plane API

The SDK provides typed, lower-level access to the control plane using
`client_credentials` (service account) authentication — for reads, ad-hoc
management, and anything not covered by the attribute sync above.

### Event Types

```php
use FlowCatalyst\Facades\FlowCatalyst;

// List event types
$result = FlowCatalyst::eventTypes()->list();
foreach ($result['items'] as $eventType) {
    echo $eventType->code . ': ' . $eventType->name;
}

// Create an event type
$eventType = FlowCatalyst::eventTypes()->create([
    'code' => 'order:fulfillment:order:created',
    'name' => 'Order Created',
    'description' => 'Fired when a new order is placed',
]);

// Add a schema version
FlowCatalyst::eventTypes()->addSchema($eventType->id, [
    'version' => '1.0',
    'mimeType' => 'application/json',
    'schema' => json_encode(['type' => 'object', 'properties' => [...]]),
    'schemaType' => 'JSON_SCHEMA',
]);

// Finalise the schema
FlowCatalyst::eventTypes()->finaliseSchema($eventType->id, '1.0');
```

### Subscriptions

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\DTOs\EventTypeBinding;
use FlowCatalyst\Enums\DispatchMode;

// Create a subscription
$subscription = FlowCatalyst::subscriptions()->create([
    'code' => 'notify-warehouse',
    'name' => 'Notify Warehouse',
    'eventTypes' => [
        ['eventTypeCode' => 'order:fulfillment:order:created'],
    ],
    'target' => 'https://warehouse.example.com/webhook',
    'queue' => 'default',
    'dispatchPoolId' => $poolId,
    'mode' => DispatchMode::IMMEDIATE,
    'timeoutSeconds' => 30,
    'maxRetries' => 5,
]);

// Pause/resume a subscription
FlowCatalyst::subscriptions()->pause($subscription->id);
FlowCatalyst::subscriptions()->resume($subscription->id);
```

### Dispatch Pools

```php
use FlowCatalyst\Facades\FlowCatalyst;

// Create a dispatch pool for rate limiting
$pool = FlowCatalyst::dispatchPools()->create([
    'code' => 'warehouse-webhooks',
    'name' => 'Warehouse Webhooks',
    'rateLimit' => 100,      // Max 100 requests per minute
    'concurrency' => 10,     // Max 10 concurrent requests
]);

// Suspend/activate a pool
FlowCatalyst::dispatchPools()->suspend($pool->id);
FlowCatalyst::dispatchPools()->activate($pool->id);
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

// List applications
$result = FlowCatalyst::applications()->list();

// Get by code
$app = FlowCatalyst::applications()->getByCode('myapp');

// Create an application
$app = FlowCatalyst::applications()->create([
    'code' => 'myapp',
    'name' => 'My Application',
    'description' => 'My awesome application',
]);
```

## Postbox (Event Creation)

The postbox allows your application to create events and dispatch jobs using the outbox pattern. Events are written to a local database table and then processed by FlowCatalyst.

### Setup

Publish and run the migration:

```bash
php artisan vendor:publish --tag=flowcatalyst-migrations
php artisan migrate
```

### Creating Events

```php
use FlowCatalyst\Facades\Postbox;
use FlowCatalyst\Postbox\DTOs\CreateEventDto;

// Create a single event
$eventId = Postbox::createEvent(
    CreateEventDto::create(
        type: 'order.created',
        data: ['orderId' => 'ORD-123', 'total' => 99.99, 'currency' => 'USD'],
        partitionId: 'orders'
    )->withCorrelationId('corr-abc-123')
      ->withSource('order-service')
);

// Batch create events
$eventIds = Postbox::createEvents([
    CreateEventDto::create('order.created', ['orderId' => 'ORD-001'], 'orders'),
    CreateEventDto::create('order.created', ['orderId' => 'ORD-002'], 'orders'),
    CreateEventDto::create('order.created', ['orderId' => 'ORD-003'], 'orders'),
]);
```

### Creating Dispatch Jobs

```php
use FlowCatalyst\Facades\Postbox;
use FlowCatalyst\Postbox\DTOs\CreateDispatchJobDto;

// Create a dispatch job (direct webhook without subscription matching)
$jobId = Postbox::createDispatchJob(
    CreateDispatchJobDto::create(
        source: 'order-service',
        code: 'notify-warehouse',
        targetUrl: 'https://warehouse.example.com/webhook',
        payload: ['orderId' => 'ORD-123', 'action' => 'prepare'],
        dispatchPoolId: $warehousePoolId,
        partitionId: 'warehouse-notifications'
    )->withCorrelationId('corr-abc-123')
      ->withHeaders(['X-Priority' => 'high'])
);
```

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
FLOWCATALYST_POSTBOX_DRIVER=mongodb
FLOWCATALYST_POSTBOX_CONNECTION=mongodb
```

Create the collection with indexes:

```javascript
db.createCollection('postbox_messages');
db.postbox_messages.createIndex(
  { tenant_id: 1, partition_id: 1, status: 1, created_at: 1 },
  { name: 'idx_postbox_pending' },
);
db.postbox_messages.createIndex({ status: 1, created_at: 1 }, { name: 'idx_postbox_status' });
```

## Error Handling

The SDK throws specific exceptions for different error types:

```php
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\AuthenticationException;
use FlowCatalyst\Exceptions\ValidationException;
use FlowCatalyst\Exceptions\PostboxException;

try {
    FlowCatalyst::eventTypes()->create([...]);
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

If you're using an AI coding agent (Claude Code, Cursor, Windsurf, etc.), you can give it read-only access to your FlowCatalyst event types, schemas, and subscriptions via the MCP server. This lets the agent explore your event catalog and generate typed code (including PHP DTOs) for you.

### Quick setup (Claude Code)

```bash
claude mcp add flowcatalyst -- npx @flowcatalyst/mcp-server
```

### Quick setup (Cursor / Windsurf / Claude Desktop)

Add to your MCP config file (`.cursor/mcp.json`, Claude Desktop config, etc.):

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "npx",
      "args": ["@flowcatalyst/mcp-server"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

You need a service account with the `AI Agent Read-Only` role. The agent can then generate PHP DTOs, TypeScript interfaces, Python dataclasses, or Java records from your event schemas. See the [MCP server README](../mcp-server/README.md) for full details.

## Testing

For testing, you can mock the facades:

```php
use FlowCatalyst\Facades\FlowCatalyst;
use FlowCatalyst\Facades\Postbox;

FlowCatalyst::shouldReceive('eventTypes->list')
    ->andReturn(['items' => [], 'total' => 0]);

Postbox::shouldReceive('createEvent')
    ->andReturn('0HZXEQ5Y8JY5Z');
```

## License

MIT License. See [LICENSE](LICENSE) for details.
