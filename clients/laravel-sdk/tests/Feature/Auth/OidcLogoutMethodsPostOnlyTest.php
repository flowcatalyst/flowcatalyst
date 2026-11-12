<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Auth;

use FlowCatalyst\FlowCatalystServiceProvider;

final class OidcLogoutMethodsPostOnlyTest extends OidcLogoutMethodsBase
{
    protected function methods(): ?array
    {
        return ['post'];
    }

    public function test_get_is_not_routed_when_post_only(): void
    {
        $this->get('/flowcatalyst/logout')->assertStatus(405);
        $this->post('/flowcatalyst/logout')->assertRedirect();
    }
}
