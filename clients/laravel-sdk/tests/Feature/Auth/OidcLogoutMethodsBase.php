<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Auth;

use FlowCatalyst\FlowCatalystServiceProvider;
use Orchestra\Testbench\TestCase;

/** L15: `oidc.logout_methods` controls which HTTP methods reach the logout route. */
abstract class OidcLogoutMethodsBase extends TestCase
{
    /** @return list<string>|null */
    abstract protected function methods(): ?array;

    protected function getPackageProviders($app): array
    {
        return [FlowCatalystServiceProvider::class];
    }

    protected function getEnvironmentSetUp($app): void
    {
        $app['config']->set('app.key', 'base64:' . base64_encode(str_repeat('a', 32)));
        $app['config']->set('session.driver', 'array');
        $app['config']->set('flowcatalyst.base_url', 'https://fc.test');
        $app['config']->set('flowcatalyst.oidc.enabled', true);
        if ($this->methods() !== null) {
            $app['config']->set('flowcatalyst.oidc.logout_methods', $this->methods());
        }
    }
}
