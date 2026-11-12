<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Auth;

use FlowCatalyst\FlowCatalystServiceProvider;

final class OidcLogoutMethodsDefaultTest extends OidcLogoutMethodsBase
{
    protected function methods(): ?array
    {
        return null;
    }

    public function test_default_accepts_get_and_post(): void
    {
        $this->get('/flowcatalyst/logout')->assertRedirect();
        $this->post('/flowcatalyst/logout')->assertRedirect();
    }
}
