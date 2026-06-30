<?php

return [
    /*
    |--------------------------------------------------------------------------
    | FlowCatalyst API Base URL
    |--------------------------------------------------------------------------
    |
    | The base URL for the FlowCatalyst platform API. This is typically your
    | FlowCatalyst instance URL.
    |
    */
    'base_url' => env('FLOWCATALYST_BASE_URL', 'https://api.flowcatalyst.io'),

    /*
    |--------------------------------------------------------------------------
    | Service Account Credentials
    |--------------------------------------------------------------------------
    |
    | These credentials are used to authenticate with the FlowCatalyst API
    | using the OAuth2 client credentials grant flow.
    |
    | For single-application deployments:
    |   Use a service account tied to your application.
    |
    | For multi-application deployments:
    |   Use a service account with access to multiple applications. Sync
    |   definitions programmatically using the DefinitionSynchronizer:
    |
    |   $synchronizer->syncAll([
    |       SyncDefinitionSet::forApplication('app-one')->withEventTypes([...]),
    |       SyncDefinitionSet::forApplication('app-two')->withEventTypes([...]),
    |   ]);
    |
    */
    'client_id' => env('FLOWCATALYST_CLIENT_ID'),
    'client_secret' => env('FLOWCATALYST_CLIENT_SECRET'),

    /*
    |--------------------------------------------------------------------------
    | Token URL
    |--------------------------------------------------------------------------
    |
    | The OAuth2 token endpoint. Defaults to {base_url}/oauth/token if not set.
    |
    */
    'token_url' => env('FLOWCATALYST_TOKEN_URL'),

    /*
    |--------------------------------------------------------------------------
    | Webhook Signing Secret
    |--------------------------------------------------------------------------
    |
    | The secret used to validate incoming webhook signatures from FlowCatalyst.
    | This should match the signing secret from your service account.
    |
    */
    'signing_secret' => env('FLOWCATALYST_SIGNING_SECRET'),

    /*
    |--------------------------------------------------------------------------
    | Token Caching
    |--------------------------------------------------------------------------
    |
    | Configuration for caching OAuth2 access tokens. The SDK automatically
    | refreshes tokens before they expire.
    |
    */
    'token_cache' => [
        'driver' => env('FLOWCATALYST_TOKEN_CACHE_DRIVER', 'file'),
        'key' => env('FLOWCATALYST_TOKEN_CACHE_KEY', 'flowcatalyst_access_token'),
    ],

    /*
    |--------------------------------------------------------------------------
    | HTTP Client Settings
    |--------------------------------------------------------------------------
    |
    | Configuration for the HTTP client used to communicate with the
    | FlowCatalyst API.
    |
    */
    'http' => [
        'timeout' => env('FLOWCATALYST_TIMEOUT', 30),
        'retry_attempts' => env('FLOWCATALYST_RETRY_ATTEMPTS', 3),
        'retry_delay' => env('FLOWCATALYST_RETRY_DELAY', 100), // milliseconds
    ],

    /*
    |--------------------------------------------------------------------------
    | OIDC User Authentication
    |--------------------------------------------------------------------------
    |
    | Configuration for authenticating users via FlowCatalyst's OIDC server.
    | This is separate from the service account credentials above which are
    | used for API access.
    |
    | User authentication flow:
    | 1. User clicks login -> redirected to FlowCatalyst
    | 2. User authenticates -> redirected back with code
    | 3. Code exchanged for tokens -> your OidcUserHandler is called
    |
    | For API access to FlowCatalyst, use the service account credentials
    | (client_id/client_secret above with client_credentials grant).
    |
    */
    'oidc' => [
        /*
        |----------------------------------------------------------------------
        | Enable OIDC User Authentication
        |----------------------------------------------------------------------
        |
        | Set to false to disable the OIDC routes entirely.
        |
        */
        'enabled' => env('FLOWCATALYST_OIDC_ENABLED', false),

        /*
        |----------------------------------------------------------------------
        | OIDC Client ID (for user authentication)
        |----------------------------------------------------------------------
        |
        | The OAuth client ID for user authentication. This is different from
        | the service account client_id used for API access.
        |
        | Create an OAuth client in FlowCatalyst with:
        | - Grant types: authorization_code
        | - Redirect URI: https://your-app.com/flowcatalyst/callback
        |
        */
        'client_id' => env('FLOWCATALYST_OIDC_CLIENT_ID'),

        /*
        |----------------------------------------------------------------------
        | OIDC Client Secret (optional for public clients)
        |----------------------------------------------------------------------
        |
        | For confidential clients, provide the client secret.
        | For public clients (SPAs), leave empty and use PKCE only.
        |
        */
        'client_secret' => env('FLOWCATALYST_OIDC_CLIENT_SECRET'),

        /*
        |----------------------------------------------------------------------
        | Requested Scopes
        |----------------------------------------------------------------------
        |
        | The scopes to request during authentication.
        | Default: openid profile email
        |
        */
        'scope' => env('FLOWCATALYST_OIDC_SCOPE', 'openid profile email'),

        /*
        |----------------------------------------------------------------------
        | Route Configuration
        |----------------------------------------------------------------------
        |
        | Customize the routes used for OIDC authentication.
        |
        */
        'login_route' => env('FLOWCATALYST_OIDC_LOGIN_ROUTE', '/flowcatalyst/login'),
        'callback_route' => env('FLOWCATALYST_OIDC_CALLBACK_ROUTE', '/flowcatalyst/callback'),
        'logout_route' => env('FLOWCATALYST_OIDC_LOGOUT_ROUTE', '/flowcatalyst/logout'),
        'refresh_route' => env('FLOWCATALYST_OIDC_REFRESH_ROUTE', '/flowcatalyst/refresh'),

        /*
        |----------------------------------------------------------------------
        | Redirect URLs
        |----------------------------------------------------------------------
        |
        | Where to redirect after login/logout.
        | These can be overridden by implementing OidcUserHandler.
        |
        */
        'redirect_after_login' => env('FLOWCATALYST_REDIRECT_AFTER_LOGIN', '/dashboard'),
        'redirect_after_logout' => env('FLOWCATALYST_REDIRECT_AFTER_LOGOUT', '/'),
        'error_redirect' => env('FLOWCATALYST_ERROR_REDIRECT', '/'),

        /*
        |----------------------------------------------------------------------
        | Native Login Bridge (use FlowCatalyst as your app's login)
        |----------------------------------------------------------------------
        |
        | These make a fresh Laravel app authenticate against FlowCatalyst with
        | (almost) zero code: stock `->middleware('auth')`, `Auth::user()`,
        | `@auth`, and policies all "just work".
        |
        | handler:
        |   'database' (default) — on OIDC callback, upsert a local user row and
        |     log them into the native Laravel guard, so the standard `auth`
        |     middleware recognises them. Still stores the SDK session principal,
        |     so the `fc.*` middleware keep working too.
        |   'session' — legacy behaviour: only store the principal in the session
        |     (no native Auth::login). Use this to preserve pre-bridge behaviour.
        |
        | Apps that need custom mapping (tenant checks, extra columns, …) bind
        | their own OidcUserHandler — that binding always wins over this default.
        |
        */
        'handler' => env('FLOWCATALYST_OIDC_HANDLER', 'database'),

        /*
        | The Eloquent user model upserted + logged in by the 'database' handler.
        | Matched by email. Defaults to a standard Laravel app's user model.
        */
        'user_model' => env('FLOWCATALYST_OIDC_USER_MODEL', \App\Models\User::class),

        /*
        | Remember the native login (sets the long-lived "remember me" cookie).
        */
        'remember_login' => env('FLOWCATALYST_OIDC_REMEMBER_LOGIN', false),

        /*
        |----------------------------------------------------------------------
        | Role Syncing (Spatie laravel-permission)
        |----------------------------------------------------------------------
        |
        | When the user model uses Spatie's HasRoles trait, the 'database'
        | handler maps the token's `roles` claim onto the local user on every
        | login. No-op if spatie/laravel-permission isn't installed.
        |
        |   sync_roles          — master switch for role syncing on login.
        |   sync_roles_mode     — 'additive' grants new roles and never removes
        |                         existing ones; 'replace' makes the token's
        |                         roles the user's authoritative full set.
        |   create_missing_roles— create local roles that don't exist yet
        |                         (off by default: only assign roles you manage).
        |   roles_guard         — Spatie guard_name to match/create roles under.
        |
        */
        'sync_roles' => env('FLOWCATALYST_OIDC_SYNC_ROLES', true),
        'sync_roles_mode' => env('FLOWCATALYST_OIDC_SYNC_ROLES_MODE', 'additive'),
        'create_missing_roles' => env('FLOWCATALYST_OIDC_CREATE_MISSING_ROLES', false),
        'roles_guard' => env('FLOWCATALYST_OIDC_ROLES_GUARD', 'web'),

        /*
        | On login, also mirror each role's PERMISSIONS from FlowCatalyst into the
        | local Spatie role (so $user->can('app:ctx:agg:act') works), not just the
        | role name. Fetched via GET /api/roles/{role} using the service account
        | and cached per role for `role_permissions_cache_ttl` seconds. Best-effort:
        | skipped silently if no service account is configured / the API is down.
        */
        'sync_role_permissions' => env('FLOWCATALYST_OIDC_SYNC_ROLE_PERMISSIONS', true),
        'role_permissions_cache_ttl' => (int) env('FLOWCATALYST_OIDC_ROLE_PERMISSIONS_CACHE_TTL', 300),

        /*
        |----------------------------------------------------------------------
        | Permission Resolver (fc-token / fc-session guards)
        |----------------------------------------------------------------------
        |
        | How a token/session principal's permissions are resolved for
        | hasPermissionTo()/can(). Independent of the Spatie/users-table sync.
        |
        |   'token'  (default) — read straight off the token's `scope` claim.
        |                        Fully stateless: no HTTP, no DB, no Spatie.
        |   'api_me'            — server-resolved via GET /api/me (one cached
        |                        HTTP call; fresher when permissions change).
        |
        | A locally-bound RbacCatalogue (offline, from roles) overrides either.
        |
        */
        'permission_resolver' => env('FLOWCATALYST_OIDC_PERMISSION_RESOLVER', 'token'),

        /*
        |----------------------------------------------------------------------
        | Auto Guest Redirect
        |----------------------------------------------------------------------
        |
        | When enabled (and OIDC is enabled), unauthenticated requests to routes
        | guarded by the stock `auth` middleware are redirected straight into the
        | FlowCatalyst login flow — so you never have to define a `login` route
        | or a custom redirect. If your app already has its own `login` route,
        | that route is preferred and this does nothing (it won't hijack a local
        | login page). Set to false to opt out entirely.
        |
        */
        'auto_guest_redirect' => env('FLOWCATALYST_OIDC_AUTO_GUEST_REDIRECT', true),

        /*
        |----------------------------------------------------------------------
        | Single Logout (RP-Initiated Logout)
        |----------------------------------------------------------------------
        |
        | When true, logout redirects through FlowCatalyst's end-session
        | endpoint so the user is signed out of the IdP too — not just this
        | app. The `redirect_after_logout` URL is sent as post_logout_redirect_uri
        | and MUST be registered in this client's `postLogoutRedirectUris`
        | whitelist on the platform (exact, or a subdomain wildcard such as
        | https://*.inhanceapps.com). Defaults to off to preserve local-only
        | logout behaviour.
        |
        */
        'single_logout' => env('FLOWCATALYST_OIDC_SINGLE_LOGOUT', false),

        /*
        |----------------------------------------------------------------------
        | Service-token fallback middleware
        |----------------------------------------------------------------------
        |
        | Used by the `fc.or-passport` middleware: a request that is NOT a valid
        | FlowCatalyst access token is handed to this middleware chain instead
        | (e.g. your existing Passport client-credentials guard). FlowCatalyst
        | tokens never touch it. Leave empty for FlowCatalyst-only routes.
        |
        */
        'token_fallback_middleware' => [
            // 'client', 'passport.app.identity',
        ],

        /*
        |----------------------------------------------------------------------
        | Service-token permission resolution (GET /api/me)
        |----------------------------------------------------------------------
        |
        | The token guard fetches the caller's server-resolved roles +
        | permissions from /api/me and caches them keyed by the access token for
        | this many seconds (one HTTP call per token, then cached). A short TTL
        | keeps revocation/role changes responsive. If /api/me is unreachable
        | the guard falls back to a bound RbacCatalogue (if any).
        |
        */
        'me_cache_ttl_seconds' => (int) env('FLOWCATALYST_OIDC_ME_CACHE_TTL', 60),
        'me_cache_driver' => env('FLOWCATALYST_OIDC_ME_CACHE_DRIVER'),

        /*
        |----------------------------------------------------------------------
        | Route Middleware
        |----------------------------------------------------------------------
        |
        | Middleware to apply to the OIDC routes. The 'web' middleware is
        | required for session handling (PKCE state storage).
        |
        | Note: Auth middleware (auth, auth:sanctum, etc.) is automatically
        | excluded from these routes since they ARE the authentication
        | mechanism. If you have custom auth middleware causing issues,
        | add it to 'exclude_middleware' below.
        |
        */
        'middleware' => ['web'],

        /*
        |----------------------------------------------------------------------
        | Excluded Middleware
        |----------------------------------------------------------------------
        |
        | Additional middleware to exclude from the OIDC routes. The SDK
        | automatically excludes: auth, auth:sanctum, auth:api, auth:web.
        |
        | Add any custom auth middleware here that should not run on the
        | login/callback/logout routes.
        |
        */
        'exclude_middleware' => [],
    ],

    /*
    |--------------------------------------------------------------------------
    | Application Code (Single-App Deployments)
    |--------------------------------------------------------------------------
    |
    | The unique code for your application in FlowCatalyst. Used by the
    | `flowcatalyst:sync` artisan command when syncing definitions.
    |
    | This is the DEFAULT application code. For a codebase that defines
    | definitions for MORE THAN ONE application, you don't have to drop to the
    | programmatic API — `flowcatalyst:sync` resolves each definition's app per
    | this order and syncs each application separately:
    |
    |   1. an explicit `application:` on the attribute (e.g. #[AsEventType]),
    |   2. a namespace match in `definitions.application_map` below,
    |   3. this default.
    |
    */
    'application_code' => env('FLOWCATALYST_APP_CODE'),

    /*
    |--------------------------------------------------------------------------
    | Definition Scanning
    |--------------------------------------------------------------------------
    |
    | Configuration for scanning PHP classes with FlowCatalyst attributes
    | (#[AsRole], #[AsEventType], #[AsSubscription]) and caching them for
    | syncing to the platform.
    |
    */
    'definitions' => [
        /*
        |----------------------------------------------------------------------
        | Application Map (Multi-Application Codebases)
        |----------------------------------------------------------------------
        |
        | Maps a class-namespace PREFIX to a FlowCatalyst application code. When
        | a scanned definition's class falls under a prefix, it syncs to that
        | application instead of the default `application_code`. Longest prefix
        | wins; an explicit `application:` on the attribute beats the map.
        |
        | Use this when one codebase owns multiple apps, or when a package ships
        | its own definitions and is its own application — the CONSUMER maps the
        | package's namespace to whatever app code they registered:
        |
        |   'application_map' => [
        |       'App\\Orders\\'           => 'orders',
        |       'Vendor\\Logistics\\'     => 'logistics',
        |   ],
        |
        */
        'application_map' => [],

        /*
        |----------------------------------------------------------------------
        | Scan Paths
        |----------------------------------------------------------------------
        |
        | Directories to scan for FlowCatalyst definition classes. These are
        | PHP classes with attributes like #[AsRole], #[AsEventType], or
        | #[AsSubscription].
        |
        | Default: app/FlowCatalyst
        |
        */
        'paths' => [
            app_path('FlowCatalyst'),
        ],

        /*
        |----------------------------------------------------------------------
        | Cache Path
        |----------------------------------------------------------------------
        |
        | Directory where scanned definitions are cached as JSON. The cache
        | is used by the sync command to avoid rescanning on every sync.
        |
        | Default: storage/flowcatalyst
        |
        */
        'cache_path' => storage_path('flowcatalyst'),

        /*
        |----------------------------------------------------------------------
        | Seed Spatie on Sync
        |----------------------------------------------------------------------
        |
        | When true, `php artisan flowcatalyst:sync` also mirrors the scanned
        | roles + permissions into the local spatie/laravel-permission tables
        | (under flowcatalyst.oidc.roles_guard), so the app's local authorization
        | model matches what it pushes to the platform. No-op if Spatie isn't
        | installed. Pass --no-spatie to skip for a single run.
        |
        */
        'seed_spatie' => env('FLOWCATALYST_SEED_SPATIE', true),
    ],

    /*
    |--------------------------------------------------------------------------
    | Outbox Configuration
    |--------------------------------------------------------------------------
    |
    | The outbox allows your application to write events and dispatch jobs
    | directly to a local database table, implementing the transactional
    | outbox pattern. The outbox processor will poll this table and send
    | messages to FlowCatalyst.
    |
    */
    'outbox' => [
        /*
        |----------------------------------------------------------------------
        | Enable Outbox
        |----------------------------------------------------------------------
        |
        | Set to false to disable the outbox functionality entirely.
        |
        */
        'enabled' => env('FLOWCATALYST_OUTBOX_ENABLED', true),

        /*
        |----------------------------------------------------------------------
        | Outbox Driver
        |----------------------------------------------------------------------
        |
        | The driver to use for storing outbox messages.
        | Supported: "database" (MySQL 8.0+, PostgreSQL 12+), "mongodb"
        |
        */
        'driver' => env('FLOWCATALYST_OUTBOX_DRIVER', 'database'),

        /*
        |----------------------------------------------------------------------
        | Database Connection
        |----------------------------------------------------------------------
        |
        | The database connection to use for the outbox. Leave null to use
        | the default connection.
        |
        */
        'connection' => env('FLOWCATALYST_OUTBOX_CONNECTION'),

        /*
        |----------------------------------------------------------------------
        | Table/Collection Name
        |----------------------------------------------------------------------
        |
        | The name of the table (or MongoDB collection) for outbox messages.
        |
        */
        'table' => env('FLOWCATALYST_OUTBOX_TABLE', 'outbox_messages'),

        /*
        |----------------------------------------------------------------------
        | Tenant ID
        |----------------------------------------------------------------------
        |
        | Your FlowCatalyst tenant ID. This is required for the outbox to
        | function correctly.
        |
        */
        'tenant_id' => env('FLOWCATALYST_TENANT_ID'),

        /*
        |----------------------------------------------------------------------
        | Audit Logging
        |----------------------------------------------------------------------
        |
        | When enabled, the use-case envelope (Operation + Runner / UnitOfWork)
        | writes an audit-log row to the outbox alongside each domain event it
        | commits — forwarded to the platform's audit log.
        |
        */
        'audit_enabled' => env('FLOWCATALYST_OUTBOX_AUDIT_ENABLED', false),

        /*
        |----------------------------------------------------------------------
        | Outbox Client
        |----------------------------------------------------------------------
        |
        | The FlowCatalyst client that owns outbox rows (the `client_id` column).
        | Defaults to the top-level `client_id` (FLOWCATALYST_CLIENT_ID) when
        | unset. The legacy `tenant_id` above is no longer used by the outbox.
        |
        */
        'client_id' => env('FLOWCATALYST_OUTBOX_CLIENT_ID'),

        /*
        |----------------------------------------------------------------------
        | Default Partition
        |----------------------------------------------------------------------
        |
        | The default partition ID to use when none is specified.
        |
        */
        'default_partition' => env('FLOWCATALYST_DEFAULT_PARTITION', 'default'),

        /*
        |----------------------------------------------------------------------
        | Strict Transactional Outbox
        |----------------------------------------------------------------------
        |
        | When enabled, the database outbox driver throws an OutboxException
        | if a write is attempted outside of an active database transaction
        | on the configured connection. This catches the most common
        | transactional-outbox bug: writing the outbox row without wrapping
        | it in `DB::transaction(fn () => ...)` alongside the business writes,
        | which lets business state and outbox state drift on partial failure.
        |
        | When disabled (default, for backwards compatibility), a warning is
        | logged via Log::warning(...) but the write proceeds. Flip this on
        | per environment once your callers consistently wrap outbox writes
        | in DB::transaction(...).
        |
        */
        'strict_transactions' => env('FLOWCATALYST_OUTBOX_STRICT_TRANSACTIONS', false),
    ],
];
