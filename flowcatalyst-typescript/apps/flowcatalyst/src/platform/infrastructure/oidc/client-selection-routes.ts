/**
 * Client Selection Routes (/auth/client)
 *
 * Endpoints for client context switching:
 * 1. List accessible clients
 * 2. Switch client context (get new token with client claim)
 * 3. Get current client context
 *
 * Useful for:
 * - Multi-client users (consultants, support staff)
 * - Anchor domain users (global access)
 * - Partner users with cross-client access grants
 */

import type { FastifyInstance } from 'fastify';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { ClientRepository } from '../persistence/repositories/client-repository.js';
import type { EmailDomainMappingRepository } from '../persistence/repositories/email-domain-mapping-repository.js';
import type { SessionCookieConfig } from './auth-routes.js';
import { getMappingAccessibleClientIds } from '../../domain/email-domain-mapping/email-domain-mapping.js';
import { permissionRegistry } from '../../authorization/permission-registry.js';

/**
 * Dependencies for client selection routes.
 */
export interface ClientSelectionDeps {
  principalRepository: PrincipalRepository;
  clientRepository: ClientRepository;
  emailDomainMappingRepository: EmailDomainMappingRepository;
  issueSessionToken: (
    principalId: string,
    email: string,
    roles: string[],
    clients: string[],
  ) => Promise<string>;
  validateSessionToken: (token: string) => Promise<string | null>;
  cookieConfig: SessionCookieConfig;
}

// ─── DTOs ───────────────────────────────────────────────────────────────────

interface ClientInfo {
  id: string;
  name: string;
  identifier: string;
}

interface AccessibleClientsResponse {
  clients: ClientInfo[];
  currentClientId: string | null;
  globalAccess: boolean;
}

interface SwitchClientRequest {
  clientId: string;
}

interface SwitchClientResponse {
  token: string;
  client: ClientInfo;
  roles: string[];
  permissions: string[];
}

interface CurrentClientResponse {
  client: ClientInfo | null;
  noClientContext: boolean;
}

/**
 * Register client selection routes.
 */
export async function registerClientSelectionRoutes(
  fastify: FastifyInstance,
  deps: ClientSelectionDeps,
): Promise<void> {
  const {
    principalRepository,
    clientRepository,
    emailDomainMappingRepository,
    issueSessionToken,
    validateSessionToken,
    cookieConfig,
  } = deps;

  /**
   * GET /auth/client/accessible
   * List accessible clients for the current user.
   */
  fastify.get('/auth/client/accessible', async (request, reply) => {
    const principal = await authenticatePrincipal(
      request,
      validateSessionToken,
      principalRepository,
      cookieConfig,
    );
    if (!principal) {
      return reply.status(401).send({ error: 'Not authenticated' });
    }

    if (!principal.active) {
      return reply.status(401).send({ error: 'User not found or inactive' });
    }

    // Determine accessible client IDs
    const accessibleClientIds = await getAccessibleClientIds(
      principal,
      emailDomainMappingRepository,
    );

    // Load client details
    let clients: ClientInfo[];
    if (accessibleClientIds === null) {
      // Global access - load all clients
      const allClients = await clientRepository.findAll();
      clients = allClients.map(toClientInfo);
    } else {
      const results = await Promise.all(
        accessibleClientIds.map((id) => clientRepository.findById(id)),
      );
      clients = results.filter((c): c is NonNullable<typeof c> => c != null).map(toClientInfo);
    }

    const globalAccess = principal.scope === 'ANCHOR';

    const response: AccessibleClientsResponse = {
      clients,
      currentClientId: principal.clientId,
      globalAccess,
    };

    return reply.send(response);
  });

  /**
   * POST /auth/client/switch
   * Switch to a different client context. Issues a new token.
   */
  fastify.post<{ Body: SwitchClientRequest }>('/auth/client/switch', async (request, reply) => {
    const principal = await authenticatePrincipal(
      request,
      validateSessionToken,
      principalRepository,
      cookieConfig,
    );
    if (!principal) {
      return reply.status(401).send({ error: 'Not authenticated' });
    }

    if (!principal.active) {
      return reply.status(401).send({ error: 'User not found or inactive' });
    }

    const targetClientId = request.body?.clientId;
    if (!targetClientId) {
      return reply.status(400).send({ error: 'clientId is required' });
    }

    // Verify access to requested client
    const accessibleClientIds = await getAccessibleClientIds(
      principal,
      emailDomainMappingRepository,
    );
    const hasAccess = accessibleClientIds === null || accessibleClientIds.includes(targetClientId);

    if (!hasAccess) {
      fastify.log.warn(
        { principalId: principal.id, targetClientId },
        'Principal attempted to switch to unauthorized client',
      );
      return reply.status(403).send({ error: 'Access denied to client' });
    }

    // Get client info
    const client = await clientRepository.findById(targetClientId);
    if (!client) {
      return reply.status(404).send({ error: 'Client not found' });
    }

    // Load roles
    const roles = principal.roles.map((r) => r.roleName);

    // Resolve permissions from roles
    const permissions: string[] = [];
    for (const roleName of roles) {
      const rolePermissions = permissionRegistry.getRolePermissions(roleName);
      for (const permission of rolePermissions) {
        if (!permissions.includes(permission)) {
          permissions.push(permission);
        }
      }
    }

    // Re-determine accessible clients for the new token
    const clientsForToken =
      accessibleClientIds === null
        ? ['*']
        : await formatClientEntries(accessibleClientIds, clientRepository);

    // Issue new token
    const newToken = await issueSessionToken(
      principal.id,
      principal.userIdentity?.email ?? '',
      roles,
      clientsForToken,
    );

    fastify.log.info(
      { principalId: principal.id, clientId: client.id, identifier: client.identifier },
      'Principal switched to client',
    );

    // Set session cookie if the request came via cookie auth
    const sessionToken = request.cookies[cookieConfig.name];
    if (sessionToken) {
      reply.setCookie(cookieConfig.name, newToken, {
        path: '/',
        maxAge: cookieConfig.maxAge,
        httpOnly: true,
        secure: cookieConfig.secure,
        sameSite: cookieConfig.sameSite,
      });
    }

    const response: SwitchClientResponse = {
      token: newToken,
      client: toClientInfo(client),
      roles,
      permissions,
    };

    return reply.send(response);
  });

  /**
   * GET /auth/client/current
   * Get current client context from the token.
   */
  fastify.get('/auth/client/current', async (request, reply) => {
    const principal = await authenticatePrincipal(
      request,
      validateSessionToken,
      principalRepository,
      cookieConfig,
    );
    if (!principal) {
      return reply.status(401).send({ error: 'Not authenticated' });
    }

    // Get current client from principal's home client
    const currentClientId = principal.clientId;

    let clientInfo: ClientInfo | null = null;
    if (currentClientId) {
      const client = await clientRepository.findById(currentClientId);
      if (client) {
        clientInfo = toClientInfo(client);
      }
    }

    const response: CurrentClientResponse = {
      client: clientInfo,
      noClientContext: currentClientId === null,
    };

    return reply.send(response);
  });

  fastify.log.info(
    'Client selection routes registered (/auth/client/accessible, /auth/client/switch, /auth/client/current)',
  );
}

// ─── Helper Functions ───────────────────────────────────────────────────────

/**
 * Authenticate the principal from session cookie or Bearer token.
 */
async function authenticatePrincipal(
  request: {
    cookies: Record<string, string | undefined>;
    headers: { authorization?: string | undefined };
  },
  validateSessionToken: (token: string) => Promise<string | null>,
  principalRepository: PrincipalRepository,
  cookieConfig: SessionCookieConfig,
) {
  let principalId: string | null = null;

  // Try session cookie first
  const sessionToken = request.cookies[cookieConfig.name];
  if (sessionToken) {
    principalId = await validateSessionToken(sessionToken);
  }

  // Fall back to Bearer token
  if (!principalId) {
    const authHeader = request.headers.authorization;
    if (authHeader?.startsWith('Bearer ')) {
      const token = authHeader.substring('Bearer '.length);
      principalId = await validateSessionToken(token);
    }
  }

  if (!principalId) {
    return null;
  }

  return principalRepository.findById(principalId);
}

/**
 * Get accessible client IDs for a principal.
 * Returns null for unrestricted (ANCHOR) access.
 */
async function getAccessibleClientIds(
  principal: {
    scope: string | null;
    clientId: string | null;
    userIdentity: { emailDomain: string } | null;
  },
  emailDomainMappingRepository: EmailDomainMappingRepository,
): Promise<string[] | null> {
  if (principal.scope === 'ANCHOR') {
    return null; // Unrestricted
  }

  const clientIds: string[] = [];

  // Add home client
  if (principal.clientId) {
    clientIds.push(principal.clientId);
  }

  // Add clients from email domain mapping
  if (principal.userIdentity?.emailDomain) {
    const mapping = await emailDomainMappingRepository.findByEmailDomain(
      principal.userIdentity.emailDomain,
    );
    if (mapping) {
      const mappedIds = getMappingAccessibleClientIds(mapping);
      for (const id of mappedIds) {
        if (!clientIds.includes(id)) {
          clientIds.push(id);
        }
      }
    }
  }

  return clientIds;
}

/**
 * Format client IDs as "id:identifier" entries for token claims.
 */
async function formatClientEntries(
  clientIds: string[],
  clientRepository: ClientRepository,
): Promise<string[]> {
  if (clientIds.length === 0) return [];

  const entries: string[] = [];
  for (const id of clientIds) {
    const client = await clientRepository.findById(id);
    if (client) {
      entries.push(`${id}:${client.identifier}`);
    } else {
      entries.push(id);
    }
  }
  return entries;
}

function toClientInfo(client: { id: string; name: string; identifier: string }): ClientInfo {
  return {
    id: client.id,
    name: client.name,
    identifier: client.identifier,
  };
}
