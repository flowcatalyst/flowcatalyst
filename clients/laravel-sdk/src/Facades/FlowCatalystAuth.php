<?php

declare(strict_types=1);

namespace FlowCatalyst\Facades;

use FlowCatalyst\Auth\DefaultOidcUserHandler;
use FlowCatalyst\Auth\DTOs\FlowCatalystUser;
use Illuminate\Support\Facades\Facade;

/**
 * Facade for FlowCatalyst OIDC user authentication helpers.
 *
 * @method static FlowCatalystUser|null user() Get the current FlowCatalyst user
 * @method static bool check() Check if a FlowCatalyst user is authenticated
 * @method static bool guest() Check if no FlowCatalyst user is authenticated
 *
 * @see \FlowCatalyst\Auth\DefaultOidcUserHandler
 */
class FlowCatalystAuth extends Facade
{
    /**
     * Get the current FlowCatalyst user from the session.
     */
    public static function user(): ?FlowCatalystUser
    {
        return DefaultOidcUserHandler::getCurrentUser();
    }

    /**
     * Check if a FlowCatalyst user is authenticated.
     */
    public static function check(): bool
    {
        return DefaultOidcUserHandler::isAuthenticated();
    }

    /**
     * Check if no FlowCatalyst user is authenticated.
     */
    public static function guest(): bool
    {
        return !self::check();
    }

    /**
     * Get the login URL.
     */
    public static function loginUrl(?string $returnUrl = null): string
    {
        $url = route('flowcatalyst.login');
        if ($returnUrl) {
            $url .= '?' . http_build_query(['return_url' => $returnUrl]);
        }
        return $url;
    }

    /**
     * Get the logout URL.
     */
    public static function logoutUrl(): string
    {
        return route('flowcatalyst.logout');
    }

    protected static function getFacadeAccessor(): string
    {
        // This facade uses static methods, so we don't need a real accessor
        return 'flowcatalyst.auth';
    }
}
