<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth;

use FlowCatalyst\Auth\Contracts\OidcUserHandler;
use FlowCatalyst\Auth\DTOs\FlowCatalystUser;

/**
 * Default OIDC user handler that stores user info in the session.
 *
 * This handler does NOT integrate with Laravel's Auth system or User model.
 * It simply stores the FlowCatalyst user in the session.
 *
 * For production use, you should implement OidcUserHandler and bind your own
 * implementation in a service provider:
 *
 * @example
 * ```php
 * // In AppServiceProvider::register()
 * $this->app->bind(OidcUserHandler::class, MyOidcUserHandler::class);
 * ```
 *
 * Or extend this class and override the methods you need:
 *
 * @example
 * ```php
 * class MyOidcUserHandler extends DefaultOidcUserHandler
 * {
 *     public function handleAuthenticatedUser(FlowCatalystUser $user): mixed
 *     {
 *         // Your custom logic here
 *         $localUser = User::updateOrCreate(
 *             ['email' => $user->email],
 *             ['name' => $user->name, 'fc_sub' => $user->sub]
 *         );
 *
 *         Auth::login($localUser);
 *
 *         // Still store in session for SDK access
 *         parent::handleAuthenticatedUser($user);
 *
 *         return $localUser;
 *     }
 * }
 * ```
 */
class DefaultOidcUserHandler implements OidcUserHandler
{
    public const SESSION_KEY = 'flowcatalyst_user';

    /**
     * Handle an authenticated FlowCatalyst user.
     *
     * Default implementation stores the user in the session.
     * Override this method to integrate with your User model and Auth system.
     */
    public function handleAuthenticatedUser(FlowCatalystUser $user): mixed
    {
        session()->put(self::SESSION_KEY, [
            'sub' => $user->sub,
            'email' => $user->email,
            'name' => $user->name,
            'clients' => $user->getClients(),
            'roles' => $user->getRoles(),
            'access_token' => $user->accessToken,
            'refresh_token' => $user->refreshToken,
        ]);

        return $user;
    }

    /**
     * Handle user logout.
     *
     * Default implementation clears the session data.
     * Override this method to also logout from Laravel Auth.
     */
    public function handleLogout(): void
    {
        session()->forget(self::SESSION_KEY);
        session()->invalidate();
        session()->regenerateToken();
    }

    /**
     * Get the URL to redirect to after successful login.
     *
     * Default: /dashboard
     */
    public function getPostLoginRedirect(): string
    {
        return config('flowcatalyst.oidc.redirect_after_login', '/dashboard');
    }

    /**
     * Get the URL to redirect to after logout.
     *
     * Default: /
     */
    public function getPostLogoutRedirect(): string
    {
        return config('flowcatalyst.oidc.redirect_after_logout', '/');
    }

    /**
     * Get the current FlowCatalyst user from the session.
     *
     * @return FlowCatalystUser|null The user if authenticated, null otherwise
     */
    public static function getCurrentUser(): ?FlowCatalystUser
    {
        $data = session()->get(self::SESSION_KEY);

        if ($data === null) {
            return null;
        }

        return new FlowCatalystUser(
            sub: $data['sub'],
            email: $data['email'],
            name: $data['name'],
            claims: [
                'sub' => $data['sub'],
                'email' => $data['email'],
                'name' => $data['name'],
                'clients' => $data['clients'],
                'roles' => $data['roles'],
            ],
            accessToken: $data['access_token'] ?? null,
            refreshToken: $data['refresh_token'] ?? null,
        );
    }

    /**
     * Check if a FlowCatalyst user is currently authenticated.
     */
    public static function isAuthenticated(): bool
    {
        return session()->has(self::SESSION_KEY);
    }
}
