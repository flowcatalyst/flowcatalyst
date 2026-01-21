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
    | OIDC Client Credentials
    |--------------------------------------------------------------------------
    |
    | These credentials are used to authenticate with the FlowCatalyst API
    | using the OAuth2 client credentials grant flow.
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
        | Route Middleware
        |----------------------------------------------------------------------
        |
        | Middleware to apply to the OIDC routes.
        |
        */
        'middleware' => ['web'],
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
        | Default Partition
        |----------------------------------------------------------------------
        |
        | The default partition ID to use when none is specified.
        |
        */
        'default_partition' => env('FLOWCATALYST_DEFAULT_PARTITION', 'default'),
    ],
];
