/**
 * OIDC Federation Routes
 *
 * Handles login flows where FlowCatalyst acts as an OIDC client,
 * federating authentication to external identity providers (Entra ID, Keycloak, etc.)
 *
 * Flow:
 * 1. GET /auth/oidc/login?domain=example.com - Redirects to external IDP
 * 2. User authenticates at external IDP
 * 3. GET /auth/oidc/callback?code=...&state=... - Handles callback, creates session
 */

import type { FastifyInstance } from 'fastify';
import * as oidcClient from 'openid-client';

import { ExecutionContext, Result, type UnitOfWork } from '@flowcatalyst/domain-core';

import type { IdentityProvider } from '../../domain/identity-provider/identity-provider.js';
import type { EmailDomainMapping } from '../../domain/email-domain-mapping/email-domain-mapping.js';
import {
  isValidIssuer,
  isEmailDomainAllowed,
} from '../../domain/identity-provider/identity-provider.js';
import { getMappingAccessibleClientIds } from '../../domain/email-domain-mapping/email-domain-mapping.js';

import type { IdentityProviderRepository } from '../persistence/repositories/identity-provider-repository.js';
import type { EmailDomainMappingRepository } from '../persistence/repositories/email-domain-mapping-repository.js';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { ClientRepository } from '../persistence/repositories/client-repository.js';
import type { RoleRepository } from '../persistence/repositories/role-repository.js';
import type { IdpRoleMappingRepository } from '../persistence/repositories/idp-role-mapping-repository.js';
import type {
  OidcLoginStateRepository,
  OidcLoginState,
} from '../persistence/repositories/oidc-login-state-repository.js';

import { createOrUpdateOidcUser, syncIdpRoles, extractIdpRoles } from './oidc-sync-service.js';
import { getAllowedRoleNames } from '../../authorization/allowed-role-filter.js';
import type { SessionCookieConfig } from './auth-routes.js';
import type { PrincipalScope } from '../../domain/principal/principal-scope.js';

export interface OidcFederationDeps {
  identityProviderRepository: IdentityProviderRepository;
  emailDomainMappingRepository: EmailDomainMappingRepository;
  principalRepository: PrincipalRepository;
  clientRepository: ClientRepository;
  roleRepository: RoleRepository;
  idpRoleMappingRepository: IdpRoleMappingRepository;
  oidcLoginStateRepository: OidcLoginStateRepository;
  unitOfWork: UnitOfWork;
  resolveClientSecret: (idp: IdentityProvider) => Promise<string | undefined>;
  issueSessionToken: (
    principalId: string,
    email: string,
    roles: string[],
    clients: string[],
  ) => Promise<string>;
  cookieConfig: SessionCookieConfig;
  externalBaseUrl: string;
}

// Discovery cache to avoid repeated metadata fetches
const discoveryCache = new Map<string, { config: oidcClient.Configuration; expiry: number }>();
const DISCOVERY_CACHE_TTL_MS = 15 * 60 * 1000; // 15 minutes

/**
 * Register OIDC federation routes on Fastify.
 */
export async function registerOidcFederationRoutes(
  fastify: FastifyInstance,
  deps: OidcFederationDeps,
): Promise<void> {
  const callbackUrl = `${deps.externalBaseUrl}/auth/oidc/callback`;

  // ==================== Login Initiation ====================

  fastify.get<{
    Querystring: {
      domain?: string;
      return_url?: string;
      oauth_client_id?: string;
      oauth_redirect_uri?: string;
      oauth_scope?: string;
      oauth_state?: string;
      oauth_code_challenge?: string;
      oauth_code_challenge_method?: string;
      oauth_nonce?: string;
    };
  }>('/auth/oidc/login', async (request, reply) => {
    const { domain: rawDomain, return_url: returnUrl } = request.query;

    if (!rawDomain) {
      return reply.status(400).send({ error: 'domain parameter is required' });
    }

    const domain = rawDomain.toLowerCase().trim();

    // Look up email domain mapping
    const mapping = await deps.emailDomainMappingRepository.findByEmailDomain(domain);
    if (!mapping) {
      return reply
        .status(404)
        .send({ error: `No authentication configuration found for domain: ${domain}` });
    }

    // Load identity provider
    const idp = await deps.identityProviderRepository.findById(mapping.identityProviderId);
    if (!idp) {
      return reply.status(404).send({ error: `Identity provider not found for domain: ${domain}` });
    }

    if (idp.type !== 'OIDC') {
      return reply
        .status(400)
        .send({ error: `Domain ${domain} uses internal authentication, not OIDC` });
    }

    if (!idp.oidcIssuerUrl || !idp.oidcClientId) {
      return reply
        .status(500)
        .send({ error: `OIDC configuration incomplete for domain: ${domain}` });
    }

    // Validate email domain is allowed by IDP
    if (!isEmailDomainAllowed(idp, domain)) {
      return reply
        .status(403)
        .send({ error: `Email domain ${domain} is not allowed by this identity provider` });
    }

    // Generate PKCE code verifier and challenge
    const codeVerifier = oidcClient.randomPKCECodeVerifier();
    const codeChallenge = await oidcClient.calculatePKCECodeChallenge(codeVerifier);

    // Generate state and nonce
    const state = oidcClient.randomState();
    const nonce = oidcClient.randomNonce();

    // Store login state
    const loginState: OidcLoginState = {
      state,
      emailDomain: domain,
      identityProviderId: idp.id,
      emailDomainMappingId: mapping.id,
      nonce,
      codeVerifier,
      returnUrl: returnUrl ?? null,
      oauthClientId: request.query.oauth_client_id ?? null,
      oauthRedirectUri: request.query.oauth_redirect_uri ?? null,
      oauthScope: request.query.oauth_scope ?? null,
      oauthState: request.query.oauth_state ?? null,
      oauthCodeChallenge: request.query.oauth_code_challenge ?? null,
      oauthCodeChallengeMethod: request.query.oauth_code_challenge_method ?? null,
      oauthNonce: request.query.oauth_nonce ?? null,
      createdAt: new Date(),
      expiresAt: new Date(Date.now() + 10 * 60 * 1000), // 10 minutes
    };

    await deps.oidcLoginStateRepository.persist(loginState);

    // Discover IDP configuration and build authorization URL
    const config = await getDiscoveryConfig(idp, deps);

    const authorizationUrl = oidcClient.buildAuthorizationUrl(config, {
      redirect_uri: callbackUrl,
      scope: 'openid profile email',
      state,
      nonce,
      code_challenge: codeChallenge,
      code_challenge_method: 'S256',
    });

    fastify.log.info({ domain, issuerUrl: idp.oidcIssuerUrl }, 'Redirecting to OIDC provider');

    return reply.redirect(authorizationUrl.href);
  });

  // ==================== Callback Handler ====================

  fastify.get<{
    Querystring: {
      code?: string;
      state?: string;
      error?: string;
      error_description?: string;
    };
  }>('/auth/oidc/callback', async (request, reply) => {
    const { code, state, error, error_description: errorDescription } = request.query;

    // Handle IDP errors
    if (error) {
      fastify.log.warn({ error, errorDescription }, 'OIDC callback error');
      return errorRedirect(reply, deps, errorDescription ?? error);
    }

    if (!code) {
      return errorRedirect(reply, deps, 'No authorization code received');
    }

    if (!state) {
      return errorRedirect(reply, deps, 'No state parameter received');
    }

    // Validate state
    const loginState = await deps.oidcLoginStateRepository.findValidState(state);
    if (!loginState) {
      fastify.log.warn({ state }, 'Invalid or expired OIDC state');
      return errorRedirect(reply, deps, 'Invalid or expired login session. Please try again.');
    }

    // Delete state immediately (single-use)
    await deps.oidcLoginStateRepository.deleteByState(state);

    // Load identity provider and mapping
    const idp = await deps.identityProviderRepository.findById(loginState.identityProviderId);
    if (!idp) {
      return errorRedirect(reply, deps, 'Identity provider no longer exists');
    }

    const mapping = await deps.emailDomainMappingRepository.findById(
      loginState.emailDomainMappingId,
    );
    if (!mapping) {
      return errorRedirect(reply, deps, 'Email domain mapping no longer exists');
    }

    try {
      // Discover IDP config and exchange code for tokens
      const config = await getDiscoveryConfig(idp, deps);

      const currentUrl = new URL(
        `${callbackUrl}?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`,
      );

      const tokens = await oidcClient.authorizationCodeGrant(config, currentUrl, {
        pkceCodeVerifier: loginState.codeVerifier,
        expectedState: state,
        expectedNonce: loginState.nonce,
      });

      // Get ID token claims
      const claims = tokens.claims();
      if (!claims) {
        return errorRedirect(reply, deps, 'No ID token received from identity provider');
      }

      // Extract required claims (use bracket notation for index signature properties)
      const rawEmail = (claims['email'] as string) ?? (claims['preferred_username'] as string);
      const email = rawEmail?.toLowerCase();
      if (!email) {
        return errorRedirect(reply, deps, 'No email claim in ID token');
      }

      const name = claims['name'] as string | undefined;
      const subject = claims.sub;
      const tenantId = claims['tid'] as string | undefined;
      const tokenIssuer = claims.iss;

      // Validate issuer
      if (!isValidIssuer(idp, tokenIssuer)) {
        fastify.log.warn({ expected: idp.oidcIssuerUrl, got: tokenIssuer }, 'Invalid token issuer');
        return errorRedirect(reply, deps, 'Invalid token issuer');
      }

      // Validate email domain matches
      const emailDomain = extractEmailDomain(email);
      if (emailDomain !== loginState.emailDomain) {
        fastify.log.warn(
          { expected: loginState.emailDomain, got: emailDomain },
          'Email domain mismatch',
        );
        return errorRedirect(reply, deps, 'Email domain does not match the login request');
      }

      // Validate email domain is allowed by IDP
      if (!isEmailDomainAllowed(idp, emailDomain)) {
        fastify.log.warn({ emailDomain, idpCode: idp.code }, 'Email domain not allowed by IDP');
        return errorRedirect(reply, deps, 'Email domain is not allowed by this identity provider');
      }

      // Verify mapping IDP matches
      if (mapping.identityProviderId !== idp.id) {
        fastify.log.warn(
          { mappingIdp: mapping.identityProviderId, usedIdp: idp.id },
          'IDP mismatch',
        );
        return errorRedirect(reply, deps, 'Identity provider configuration mismatch');
      }

      // Validate OIDC tenant ID for multi-tenant identity providers
      if (idp.oidcMultiTenant) {
        if (!mapping.requiredOidcTenantId) {
          fastify.log.error(
            { idpCode: idp.code, emailDomain },
            'SECURITY: Multi-tenant IDP missing requiredOidcTenantId',
          );
          return errorRedirect(
            reply,
            deps,
            'Configuration error: tenant ID required for this identity provider',
          );
        }
        if (!tenantId) {
          fastify.log.warn({ emailDomain }, 'Tenant ID required but not in token');
          return errorRedirect(
            reply,
            deps,
            'Authentication failed: tenant information not provided',
          );
        }
        if (mapping.requiredOidcTenantId !== tenantId) {
          fastify.log.warn(
            { emailDomain, expected: mapping.requiredOidcTenantId, got: tenantId },
            'Tenant ID mismatch',
          );
          return errorRedirect(reply, deps, 'Authentication failed: unauthorized tenant');
        }
      } else if (mapping.requiredOidcTenantId) {
        // Single-tenant IDP with optional tenant ID check
        if (tenantId && mapping.requiredOidcTenantId !== tenantId) {
          fastify.log.warn(
            { emailDomain, expected: mapping.requiredOidcTenantId, got: tenantId },
            'Tenant ID mismatch',
          );
          return errorRedirect(reply, deps, 'Authentication failed: unauthorized tenant');
        }
      }

      // Map scopeType to PrincipalScope
      const userScope = mapping.scopeType as PrincipalScope;

      // Determine client ID for user
      const userClientId = mapping.scopeType === 'CLIENT' ? mapping.primaryClientId : null;

      // Create execution context for audit trail (system-initiated OIDC operation)
      const ctx = ExecutionContext.create('system:oidc-federation');

      const syncDeps = {
        principalRepository: deps.principalRepository,
        idpRoleMappingRepository: deps.idpRoleMappingRepository,
        unitOfWork: deps.unitOfWork,
        log: fastify.log,
      };

      // Find or create user (via UnitOfWork for audit log + domain event)
      const userResult = await createOrUpdateOidcUser(
        {
          email,
          name: name ?? null,
          externalIdpId: subject,
          clientId: userClientId,
          scope: userScope,
        },
        ctx,
        syncDeps,
      );

      if (Result.isFailure(userResult)) {
        fastify.log.error({ error: userResult.error }, 'Failed to create/update OIDC user');
        return errorRedirect(reply, deps, 'Failed to create user account');
      }

      // Re-read principal after commit to get full entity
      const userId = userResult.value.getData().userId;
      let principal = await deps.principalRepository.findById(userId);
      if (!principal) {
        return errorRedirect(reply, deps, 'Failed to load user after creation');
      }

      // Sync IDP roles if enabled for this domain mapping
      if (mapping.syncRolesFromIdp) {
        const idTokenPayload = claims as unknown as Record<string, unknown>;
        const idpRoleNames = extractIdpRoles(idTokenPayload);
        if (idpRoleNames.length > 0) {
          const allowedNames = await getAllowedRoleNames(mapping.emailDomain, {
            emailDomainMappingRepository: deps.emailDomainMappingRepository,
            roleRepository: deps.roleRepository,
          });

          const roleResult = await syncIdpRoles(
            principal,
            idpRoleNames,
            allowedNames,
            ctx,
            syncDeps,
          );

          if (Result.isFailure(roleResult)) {
            fastify.log.error({ error: roleResult.error }, 'Failed to sync IDP roles');
            // Continue with login even if role sync fails - user was created successfully
          }

          // Re-read principal to get updated roles after sync
          principal = (await deps.principalRepository.findById(principal.id)) ?? principal;
        }
      }

      // Load roles
      const roles = principal.roles.map((r) => r.roleName);

      // Determine accessible clients based on scope and mapping
      const clients = await determineAccessibleClients(principal, roles, mapping, deps);

      // Issue session token
      const sessionToken = await deps.issueSessionToken(principal.id, email, roles, clients);

      // Set session cookie
      reply.setCookie(deps.cookieConfig.name, sessionToken, {
        path: '/',
        maxAge: deps.cookieConfig.maxAge,
        httpOnly: true,
        secure: deps.cookieConfig.secure,
        sameSite: deps.cookieConfig.sameSite,
      });

      // Determine redirect URL
      const redirectUrl = determineRedirectUrl(loginState, deps.externalBaseUrl);

      fastify.log.info(
        { email, principalId: principal.id, idpCode: idp.code },
        'OIDC login successful',
      );

      return reply.redirect(redirectUrl);
    } catch (err) {
      fastify.log.error(
        { err, emailDomain: loginState.emailDomain },
        'OIDC callback processing failed',
      );
      return errorRedirect(reply, deps, 'Authentication failed. Please try again.');
    }
  });

  fastify.log.info('OIDC federation routes registered (/auth/oidc/login, /auth/oidc/callback)');
}

// ==================== Helper Functions ====================

/**
 * Get or create an openid-client Configuration via discovery.
 */
async function getDiscoveryConfig(
  idp: IdentityProvider,
  deps: OidcFederationDeps,
): Promise<oidcClient.Configuration> {
  const cacheKey = idp.oidcIssuerUrl!;
  const cached = discoveryCache.get(cacheKey);

  if (cached && cached.expiry > Date.now()) {
    return cached.config;
  }

  // Resolve client secret if available
  const clientSecret = await deps.resolveClientSecret(idp);

  const issuer = new URL(idp.oidcIssuerUrl!);
  const config = await oidcClient.discovery(issuer, idp.oidcClientId!, clientSecret);

  discoveryCache.set(cacheKey, {
    config,
    expiry: Date.now() + DISCOVERY_CACHE_TTL_MS,
  });

  return config;
}

/**
 * Determine which clients the user can access based on their scope and email domain mapping.
 */
async function determineAccessibleClients(
  principal: {
    scope: string | null;
    clientId: string | null;
    roles: readonly { roleName: string }[];
  },
  roles: string[],
  mapping: EmailDomainMapping,
  deps: OidcFederationDeps,
): Promise<string[]> {
  switch (mapping.scopeType) {
    case 'ANCHOR':
      return ['*'];
    case 'CLIENT':
    case 'PARTNER': {
      const clientIds = getMappingAccessibleClientIds(mapping);
      return formatClientEntries(clientIds, deps);
    }
  }

  // Fallback: check roles for platform admins
  if (roles.some((r) => r.includes('platform:admin') || r.includes('super-admin'))) {
    return ['*'];
  }

  if (principal.clientId) {
    return formatClientEntries([principal.clientId], deps);
  }

  return [];
}

/**
 * Format client IDs as "id:identifier" entries for the clients claim.
 */
async function formatClientEntries(
  clientIds: string[],
  deps: OidcFederationDeps,
): Promise<string[]> {
  if (clientIds.length === 0) return [];

  const entries: string[] = [];
  for (const id of clientIds) {
    const client = await deps.clientRepository.findById(id);
    if (client && 'identifier' in client && client.identifier) {
      entries.push(`${id}:${client.identifier}`);
    } else {
      entries.push(id);
    }
  }
  return entries;
}

/**
 * Determine where to redirect after successful login.
 */
function determineRedirectUrl(loginState: OidcLoginState, baseUrl: string): string {
  // If this was part of an OAuth flow, redirect back to authorize endpoint
  if (loginState.oauthClientId) {
    const params = new URLSearchParams();
    params.set('response_type', 'code');
    params.set('client_id', loginState.oauthClientId);
    if (loginState.oauthRedirectUri) params.set('redirect_uri', loginState.oauthRedirectUri);
    if (loginState.oauthScope) params.set('scope', loginState.oauthScope);
    if (loginState.oauthState) params.set('state', loginState.oauthState);
    if (loginState.oauthCodeChallenge) params.set('code_challenge', loginState.oauthCodeChallenge);
    if (loginState.oauthCodeChallengeMethod)
      params.set('code_challenge_method', loginState.oauthCodeChallengeMethod);
    if (loginState.oauthNonce) params.set('nonce', loginState.oauthNonce);
    return `${baseUrl}/oauth/authorize?${params.toString()}`;
  }

  // Return to specified URL or default to dashboard
  if (loginState.returnUrl) {
    if (loginState.returnUrl.startsWith('/')) {
      return `${baseUrl}${loginState.returnUrl}`;
    }
    return loginState.returnUrl;
  }

  return `${baseUrl}/dashboard`;
}

/**
 * Extract the email domain from an email address.
 */
function extractEmailDomain(email: string): string {
  const atIndex = email.indexOf('@');
  if (atIndex === -1) return '';
  return email.substring(atIndex + 1).toLowerCase();
}

/**
 * Redirect to frontend error page.
 */
function errorRedirect(
  reply: { redirect: (url: string) => void },
  deps: OidcFederationDeps,
  message: string,
) {
  return reply.redirect(`/?error=${encodeURIComponent(message)}`);
}
