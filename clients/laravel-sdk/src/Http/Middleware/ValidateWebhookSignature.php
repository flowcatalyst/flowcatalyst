<?php

declare(strict_types=1);

namespace FlowCatalyst\Http\Middleware;

use Closure;
use FlowCatalyst\Exceptions\WebhookValidationException;
use FlowCatalyst\Webhook\WebhookValidator;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Middleware to validate FlowCatalyst webhook signatures.
 *
 * Usage in routes:
 *   Route::post('/webhooks/flowcatalyst', [WebhookController::class, 'handle'])
 *       ->middleware('flowcatalyst.webhook');
 */
class ValidateWebhookSignature
{
    /**
     * Handle an incoming request.
     *
     * @param Closure(Request): Response $next
     */
    public function handle(Request $request, Closure $next): Response
    {
        try {
            $validator = WebhookValidator::fromConfig();
            $validator->validateRequest($request);
        } catch (WebhookValidationException $e) {
            return response()->json([
                'error' => $e->getMessage(),
            ], $e->getCode() ?: 401);
        }

        return $next($request);
    }
}
