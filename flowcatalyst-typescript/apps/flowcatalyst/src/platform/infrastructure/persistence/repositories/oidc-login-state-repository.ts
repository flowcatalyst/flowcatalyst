/**
 * OIDC Login State Repository
 *
 * Manages transient OIDC login state for the authorization code flow.
 * States are single-use and expire after 10 minutes.
 */

import { eq, lt } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { oidcLoginStates, type OidcLoginStateRecord } from '../schema/oidc-login-states.js';

export interface OidcLoginState {
  state: string;
  emailDomain: string;
  identityProviderId: string;
  emailDomainMappingId: string;
  nonce: string;
  codeVerifier: string;
  returnUrl: string | null;
  oauthClientId: string | null;
  oauthRedirectUri: string | null;
  oauthScope: string | null;
  oauthState: string | null;
  oauthCodeChallenge: string | null;
  oauthCodeChallengeMethod: string | null;
  oauthNonce: string | null;
  interactionUid: string | null;
  createdAt: Date;
  expiresAt: Date;
}

export interface OidcLoginStateRepository {
  findValidState(state: string, tx?: TransactionContext): Promise<OidcLoginState | undefined>;
  persist(loginState: OidcLoginState, tx?: TransactionContext): Promise<void>;
  deleteByState(state: string, tx?: TransactionContext): Promise<void>;
  deleteExpired(tx?: TransactionContext): Promise<void>;
}

export function createOidcLoginStateRepository(defaultDb: AnyDb): OidcLoginStateRepository {
  const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

  function hydrate(record: OidcLoginStateRecord): OidcLoginState {
    return {
      state: record.state,
      emailDomain: record.emailDomain,
      identityProviderId: record.identityProviderId,
      emailDomainMappingId: record.emailDomainMappingId,
      nonce: record.nonce,
      codeVerifier: record.codeVerifier,
      returnUrl: record.returnUrl,
      oauthClientId: record.oauthClientId,
      oauthRedirectUri: record.oauthRedirectUri,
      oauthScope: record.oauthScope,
      oauthState: record.oauthState,
      oauthCodeChallenge: record.oauthCodeChallenge,
      oauthCodeChallengeMethod: record.oauthCodeChallengeMethod,
      oauthNonce: record.oauthNonce,
      interactionUid: record.interactionUid ?? null,
      createdAt: record.createdAt,
      expiresAt: record.expiresAt,
    };
  }

  return {
    async findValidState(
      state: string,
      tx?: TransactionContext,
    ): Promise<OidcLoginState | undefined> {
      const [record] = await db(tx)
        .select()
        .from(oidcLoginStates)
        .where(eq(oidcLoginStates.state, state))
        .limit(1);

      if (!record) return undefined;

      // Check expiry
      if (record.expiresAt < new Date()) {
        return undefined;
      }

      return hydrate(record);
    },

    async persist(loginState: OidcLoginState, tx?: TransactionContext): Promise<void> {
      await db(tx).insert(oidcLoginStates).values({
        state: loginState.state,
        emailDomain: loginState.emailDomain,
        identityProviderId: loginState.identityProviderId,
        emailDomainMappingId: loginState.emailDomainMappingId,
        nonce: loginState.nonce,
        codeVerifier: loginState.codeVerifier,
        returnUrl: loginState.returnUrl,
        oauthClientId: loginState.oauthClientId,
        oauthRedirectUri: loginState.oauthRedirectUri,
        oauthScope: loginState.oauthScope,
        oauthState: loginState.oauthState,
        oauthCodeChallenge: loginState.oauthCodeChallenge,
        oauthCodeChallengeMethod: loginState.oauthCodeChallengeMethod,
        oauthNonce: loginState.oauthNonce,
        interactionUid: loginState.interactionUid,
        createdAt: loginState.createdAt,
        expiresAt: loginState.expiresAt,
      });
    },

    async deleteByState(state: string, tx?: TransactionContext): Promise<void> {
      await db(tx).delete(oidcLoginStates).where(eq(oidcLoginStates.state, state));
    },

    async deleteExpired(tx?: TransactionContext): Promise<void> {
      await db(tx).delete(oidcLoginStates).where(lt(oidcLoginStates.expiresAt, new Date()));
    },
  };
}
