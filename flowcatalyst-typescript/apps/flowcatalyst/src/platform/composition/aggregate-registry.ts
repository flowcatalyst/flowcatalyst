/**
 * Aggregate registry — prefix map and handler registration.
 */

import {
  createAggregateRegistry,
  createAggregateHandler,
} from '@flowcatalyst/persistence';
import type { Repositories } from './repositories.js';

export function createPlatformAggregateRegistry(repos: Repositories) {
  const aggregateRegistry = createAggregateRegistry({
    prn: 'Principal',
    clt: 'Client',
    anc: 'AnchorDomain',
    app: 'Application',
    apc: 'ApplicationClientConfig',
    rol: 'AuthRole',
    gnt: 'ClientAccessGrant',
    cac: 'ClientAuthConfig',
    oac: 'OAuthClient',
    evt: 'EventType',
    dpl: 'DispatchPool',
    sub: 'Subscription',
    idp: 'IdentityProvider',
    edm: 'EmailDomainMapping',
    cor: 'CorsAllowedOrigin',
  });

  aggregateRegistry.register(createAggregateHandler('Principal', repos.principalRepository));
  aggregateRegistry.register(createAggregateHandler('Client', repos.clientRepository));
  aggregateRegistry.register(createAggregateHandler('AnchorDomain', repos.anchorDomainRepository));
  aggregateRegistry.register(createAggregateHandler('Application', repos.applicationRepository));
  aggregateRegistry.register(
    createAggregateHandler('ApplicationClientConfig', repos.applicationClientConfigRepository),
  );
  aggregateRegistry.register(createAggregateHandler('AuthRole', repos.roleRepository));
  aggregateRegistry.register(
    createAggregateHandler('ClientAccessGrant', repos.clientAccessGrantRepository),
  );
  aggregateRegistry.register(
    createAggregateHandler('ClientAuthConfig', repos.clientAuthConfigRepository),
  );
  aggregateRegistry.register(createAggregateHandler('OAuthClient', repos.oauthClientRepository));
  aggregateRegistry.register(createAggregateHandler('EventType', repos.eventTypeRepository));
  aggregateRegistry.register(createAggregateHandler('DispatchPool', repos.dispatchPoolRepository));
  aggregateRegistry.register(createAggregateHandler('Subscription', repos.subscriptionRepository));
  aggregateRegistry.register(
    createAggregateHandler('IdentityProvider', repos.identityProviderRepository),
  );
  aggregateRegistry.register(
    createAggregateHandler('EmailDomainMapping', repos.emailDomainMappingRepository),
  );
  aggregateRegistry.register(
    createAggregateHandler('CorsAllowedOrigin', repos.corsAllowedOriginRepository),
  );

  return aggregateRegistry;
}
