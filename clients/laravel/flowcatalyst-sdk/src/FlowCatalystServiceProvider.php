<?php

declare(strict_types=1);

namespace FlowCatalyst;

use FlowCatalyst\Auth\Contracts\OidcUserHandler;
use FlowCatalyst\Auth\DefaultOidcUserHandler;
use FlowCatalyst\Client\Auth\OidcTokenManager;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\Drivers\DatabaseDriver;
use FlowCatalyst\Outbox\Drivers\MongoDriver;
use FlowCatalyst\Outbox\OutboxManager;
use Illuminate\Support\ServiceProvider;

class FlowCatalystServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        $this->mergeConfigFrom(
            __DIR__ . '/../config/flowcatalyst.php',
            'flowcatalyst'
        );

        $this->registerTokenManager();
        $this->registerClient();
        $this->registerOutbox();
        $this->registerOidcUserAuth();
    }

    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        $this->publishConfig();
        $this->publishMigrations();
        $this->registerMiddleware();
        $this->registerOidcRoutes();
    }

    /**
     * Register the OIDC token manager.
     */
    protected function registerTokenManager(): void
    {
        $this->app->singleton(OidcTokenManager::class, function ($app) {
            $config = $app['config']['flowcatalyst'];

            return new OidcTokenManager(
                baseUrl: $config['base_url'],
                clientId: $config['client_id'] ?? '',
                clientSecret: $config['client_secret'] ?? '',
                tokenUrl: $config['token_url'],
                cache: $app['cache']->driver($config['token_cache']['driver'] ?? null),
                cacheKey: $config['token_cache']['key'] ?? 'flowcatalyst_access_token'
            );
        });
    }

    /**
     * Register the FlowCatalyst client.
     */
    protected function registerClient(): void
    {
        $this->app->singleton(FlowCatalystClient::class, function ($app) {
            $config = $app['config']['flowcatalyst'];

            return new FlowCatalystClient(
                tokenManager: $app->make(OidcTokenManager::class),
                baseUrl: $config['base_url'],
                timeout: $config['http']['timeout'] ?? 30,
                retryAttempts: $config['http']['retry_attempts'] ?? 3,
                retryDelay: $config['http']['retry_delay'] ?? 100
            );
        });
    }

    /**
     * Register the outbox manager.
     */
    protected function registerOutbox(): void
    {
        // Register the driver based on configuration
        $this->app->singleton(OutboxDriver::class, function ($app) {
            $config = $app['config']['flowcatalyst']['outbox'];
            $driver = $config['driver'] ?? 'database';

            return match ($driver) {
                'mongodb' => new MongoDriver(
                    connection: $config['connection'],
                    collection: $config['table'] ?? 'outbox_messages'
                ),
                default => new DatabaseDriver(
                    connection: $config['connection'],
                    table: $config['table'] ?? 'outbox_messages'
                ),
            };
        });

        $this->app->singleton(OutboxManager::class, function ($app) {
            $config = $app['config']['flowcatalyst']['outbox'];

            return new OutboxManager(
                driver: $app->make(OutboxDriver::class),
                tenantId: (int) ($config['tenant_id'] ?? 0),
                defaultPartition: $config['default_partition'] ?? 'default'
            );
        });
    }

    /**
     * Publish the configuration file.
     */
    protected function publishConfig(): void
    {
        $this->publishes([
            __DIR__ . '/../config/flowcatalyst.php' => config_path('flowcatalyst.php'),
        ], 'flowcatalyst-config');
    }

    /**
     * Publish the database migrations.
     */
    protected function publishMigrations(): void
    {
        $this->publishes([
            __DIR__ . '/../database/migrations/' => database_path('migrations'),
        ], 'flowcatalyst-migrations');
    }

    /**
     * Register the webhook validation middleware.
     */
    protected function registerMiddleware(): void
    {
        $this->app['router']->aliasMiddleware(
            'flowcatalyst.webhook',
            \FlowCatalyst\Http\Middleware\ValidateWebhookSignature::class
        );
    }

    /**
     * Register the OIDC user authentication handler.
     *
     * Applications can override this by binding their own OidcUserHandler
     * implementation in their AppServiceProvider.
     */
    protected function registerOidcUserAuth(): void
    {
        // Only bind if not already bound (allows app to override)
        if (!$this->app->bound(OidcUserHandler::class)) {
            $this->app->singleton(OidcUserHandler::class, DefaultOidcUserHandler::class);
        }
    }

    /**
     * Register the OIDC authentication routes.
     */
    protected function registerOidcRoutes(): void
    {
        if (!config('flowcatalyst.oidc.enabled', false)) {
            return;
        }

        $this->app['router']->group([
            'middleware' => config('flowcatalyst.oidc.middleware', ['web']),
        ], function ($router) {
            $loginRoute = config('flowcatalyst.oidc.login_route', '/flowcatalyst/login');
            $callbackRoute = config('flowcatalyst.oidc.callback_route', '/flowcatalyst/callback');
            $logoutRoute = config('flowcatalyst.oidc.logout_route', '/flowcatalyst/logout');

            $router->get($loginRoute, [\FlowCatalyst\Auth\Http\Controllers\OidcAuthController::class, 'login'])
                ->name('flowcatalyst.login');

            $router->get($callbackRoute, [\FlowCatalyst\Auth\Http\Controllers\OidcAuthController::class, 'callback'])
                ->name('flowcatalyst.callback');

            $router->match(['get', 'post'], $logoutRoute, [\FlowCatalyst\Auth\Http\Controllers\OidcAuthController::class, 'logout'])
                ->name('flowcatalyst.logout');
        });
    }
}
