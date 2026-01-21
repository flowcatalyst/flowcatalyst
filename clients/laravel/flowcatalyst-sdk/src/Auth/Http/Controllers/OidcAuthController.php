<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Http\Controllers;

use FlowCatalyst\Auth\Contracts\OidcUserHandler;
use FlowCatalyst\Auth\DTOs\FlowCatalystUser;
use FlowCatalyst\Auth\Support\PkceGenerator;
use FlowCatalyst\Exceptions\AuthenticationException;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Routing\Controller;
use Illuminate\Support\Facades\Log;

/**
 * OIDC Authentication Controller.
 *
 * Handles the OAuth 2.0 Authorization Code flow with PKCE to authenticate
 * users against FlowCatalyst's OIDC server.
 *
 * Routes:
 * - GET /flowcatalyst/login  - Initiates OIDC login
 * - GET /flowcatalyst/callback - Handles callback from FlowCatalyst
 * - POST /flowcatalyst/logout - Logs out the user
 */
class OidcAuthController extends Controller
{
    private const STATE_SESSION_KEY = 'flowcatalyst_oidc_state';
    private const VERIFIER_SESSION_KEY = 'flowcatalyst_oidc_verifier';
    private const NONCE_SESSION_KEY = 'flowcatalyst_oidc_nonce';
    private const RETURN_URL_SESSION_KEY = 'flowcatalyst_oidc_return_url';

    private Client $httpClient;

    public function __construct(
        private readonly OidcUserHandler $userHandler
    ) {
        $this->httpClient = new Client([
            'timeout' => 30,
            'http_errors' => false,
        ]);
    }

    /**
     * Initiate OIDC login flow.
     *
     * Redirects the user to FlowCatalyst's authorization endpoint.
     */
    public function login(Request $request): RedirectResponse
    {
        $config = $this->getOidcConfig();

        // Generate PKCE values
        $codeVerifier = PkceGenerator::generateCodeVerifier();
        $codeChallenge = PkceGenerator::generateCodeChallenge($codeVerifier);
        $state = PkceGenerator::generateState();
        $nonce = PkceGenerator::generateNonce();

        // Store in session for callback validation
        session()->put(self::STATE_SESSION_KEY, $state);
        session()->put(self::VERIFIER_SESSION_KEY, $codeVerifier);
        session()->put(self::NONCE_SESSION_KEY, $nonce);

        // Store return URL if provided
        if ($request->has('return_url')) {
            session()->put(self::RETURN_URL_SESSION_KEY, $request->input('return_url'));
        }

        // Build authorization URL
        $authUrl = $this->buildAuthorizationUrl($config, $state, $nonce, $codeChallenge);

        return redirect()->away($authUrl);
    }

    /**
     * Handle OIDC callback from FlowCatalyst.
     *
     * Exchanges the authorization code for tokens and calls the user handler.
     */
    public function callback(Request $request): RedirectResponse
    {
        // Check for errors from the authorization server
        if ($request->has('error')) {
            $error = $request->input('error');
            $description = $request->input('error_description', 'Authentication failed');
            Log::warning("OIDC callback error: {$error} - {$description}");
            return $this->errorRedirect($description);
        }

        // Validate required parameters
        $code = $request->input('code');
        $state = $request->input('state');

        if (empty($code)) {
            return $this->errorRedirect('No authorization code received');
        }

        // Validate state to prevent CSRF
        $expectedState = session()->pull(self::STATE_SESSION_KEY);
        if (empty($state) || $state !== $expectedState) {
            Log::warning('OIDC state mismatch', [
                'expected' => $expectedState,
                'received' => $state,
            ]);
            return $this->errorRedirect('Invalid state parameter. Please try again.');
        }

        // Get stored PKCE verifier and nonce
        $codeVerifier = session()->pull(self::VERIFIER_SESSION_KEY);
        $nonce = session()->pull(self::NONCE_SESSION_KEY);
        $returnUrl = session()->pull(self::RETURN_URL_SESSION_KEY);

        if (empty($codeVerifier)) {
            return $this->errorRedirect('Session expired. Please try again.');
        }

        try {
            $config = $this->getOidcConfig();

            // Exchange code for tokens
            $tokens = $this->exchangeCodeForTokens($config, $code, $codeVerifier);

            // Parse and validate ID token
            $claims = $this->parseIdToken($tokens['id_token']);

            // Validate nonce
            if (!empty($nonce) && ($claims['nonce'] ?? null) !== $nonce) {
                throw new AuthenticationException('Invalid nonce in ID token');
            }

            // Create FlowCatalystUser from claims
            $fcUser = FlowCatalystUser::fromClaims(
                $claims,
                $tokens['access_token'] ?? null,
                $tokens['refresh_token'] ?? null
            );

            // Call the user handler (this is where the app customizes login)
            $this->userHandler->handleAuthenticatedUser($fcUser);

            // Redirect to post-login URL
            $redirectUrl = $returnUrl ?? $this->userHandler->getPostLoginRedirect();
            return redirect()->to($redirectUrl);

        } catch (AuthenticationException $e) {
            Log::error('OIDC authentication failed', ['error' => $e->getMessage()]);
            return $this->errorRedirect($e->getMessage());
        } catch (\Exception $e) {
            Log::error('OIDC callback error', ['error' => $e->getMessage()]);
            return $this->errorRedirect('Authentication failed. Please try again.');
        }
    }

    /**
     * Handle logout.
     *
     * Calls the user handler's logout method and redirects.
     */
    public function logout(Request $request): RedirectResponse
    {
        $this->userHandler->handleLogout();

        $redirectUrl = $this->userHandler->getPostLogoutRedirect();
        return redirect()->to($redirectUrl);
    }

    /**
     * Build the authorization URL for the OIDC server.
     */
    private function buildAuthorizationUrl(array $config, string $state, string $nonce, string $codeChallenge): string
    {
        $baseUrl = rtrim($config['base_url'], '/');
        $authorizeUrl = $baseUrl . '/oauth/authorize';

        $params = [
            'response_type' => 'code',
            'client_id' => $config['client_id'],
            'redirect_uri' => $this->getCallbackUrl(),
            'scope' => $config['scope'] ?? 'openid profile email',
            'state' => $state,
            'nonce' => $nonce,
            'code_challenge' => $codeChallenge,
            'code_challenge_method' => 'S256',
        ];

        return $authorizeUrl . '?' . http_build_query($params);
    }

    /**
     * Exchange authorization code for tokens.
     *
     * @return array{access_token: string, id_token: string, refresh_token?: string}
     * @throws AuthenticationException
     */
    private function exchangeCodeForTokens(array $config, string $code, string $codeVerifier): array
    {
        $baseUrl = rtrim($config['base_url'], '/');
        $tokenUrl = $baseUrl . '/oauth/token';

        $params = [
            'grant_type' => 'authorization_code',
            'code' => $code,
            'redirect_uri' => $this->getCallbackUrl(),
            'client_id' => $config['client_id'],
            'code_verifier' => $codeVerifier,
        ];

        // Add client secret if configured (for confidential clients)
        if (!empty($config['client_secret'])) {
            $params['client_secret'] = $config['client_secret'];
        }

        try {
            $response = $this->httpClient->post($tokenUrl, [
                'form_params' => $params,
                'headers' => [
                    'Accept' => 'application/json',
                ],
            ]);

            $statusCode = $response->getStatusCode();
            $body = json_decode((string) $response->getBody(), true);

            if ($statusCode !== 200) {
                $error = $body['error_description'] ?? $body['error'] ?? 'Token exchange failed';
                throw AuthenticationException::tokenFetchFailed($error);
            }

            if (empty($body['id_token'])) {
                throw AuthenticationException::tokenFetchFailed('No ID token received');
            }

            return $body;

        } catch (GuzzleException $e) {
            throw AuthenticationException::tokenFetchFailed($e->getMessage());
        }
    }

    /**
     * Parse and decode an ID token (JWT).
     *
     * Note: This does basic validation. For production, you should validate
     * the signature using the JWKS endpoint.
     *
     * @return array<string, mixed>
     * @throws AuthenticationException
     */
    private function parseIdToken(string $idToken): array
    {
        $parts = explode('.', $idToken);
        if (count($parts) !== 3) {
            throw new AuthenticationException('Invalid ID token format');
        }

        $payload = json_decode(base64_decode(strtr($parts[1], '-_', '+/')), true);
        if (!is_array($payload)) {
            throw new AuthenticationException('Failed to decode ID token');
        }

        // Validate expiration
        if (isset($payload['exp']) && $payload['exp'] < time()) {
            throw new AuthenticationException('ID token has expired');
        }

        // Validate required claims
        if (empty($payload['sub'])) {
            throw new AuthenticationException('Missing sub claim in ID token');
        }

        if (empty($payload['email']) && empty($payload['preferred_username'])) {
            throw new AuthenticationException('Missing email claim in ID token');
        }

        return $payload;
    }

    /**
     * Get the callback URL for this application.
     */
    private function getCallbackUrl(): string
    {
        return url(config('flowcatalyst.oidc.callback_route', '/flowcatalyst/callback'));
    }

    /**
     * Get OIDC configuration from config file.
     *
     * @return array{base_url: string, client_id: string, client_secret?: string, scope?: string}
     * @throws AuthenticationException
     */
    private function getOidcConfig(): array
    {
        $config = [
            'base_url' => config('flowcatalyst.base_url'),
            'client_id' => config('flowcatalyst.oidc.client_id'),
            'client_secret' => config('flowcatalyst.oidc.client_secret'),
            'scope' => config('flowcatalyst.oidc.scope', 'openid profile email'),
        ];

        if (empty($config['base_url'])) {
            throw AuthenticationException::missingConfiguration('FLOWCATALYST_BASE_URL');
        }

        if (empty($config['client_id'])) {
            throw AuthenticationException::missingConfiguration('FLOWCATALYST_OIDC_CLIENT_ID');
        }

        return $config;
    }

    /**
     * Redirect to error page with message.
     */
    private function errorRedirect(string $message): RedirectResponse
    {
        $errorRoute = config('flowcatalyst.oidc.error_redirect', '/');
        return redirect()->to($errorRoute)->with('error', $message);
    }
}
