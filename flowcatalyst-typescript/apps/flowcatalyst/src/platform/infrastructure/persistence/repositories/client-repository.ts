/**
 * Client Repository
 *
 * Data access for Client entities.
 */

import { eq, sql, inArray, and, or, ilike } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import {
  type PaginatedRepository,
  type PagedResult,
  type TransactionContext,
  createPagedResult,
} from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
  clients,
  type ClientRecord,
  type NewClientRecord,
  type ClientNoteJson,
} from '../schema/index.js';
import {
  type Client,
  type NewClient,
  type ClientNote,
  type ClientStatus,
} from '../../../domain/index.js';

/**
 * Client repository interface.
 */
export interface ClientRepository extends PaginatedRepository<Client> {
  findByIdentifier(identifier: string, tx?: TransactionContext): Promise<Client | undefined>;
  existsByIdentifier(identifier: string, tx?: TransactionContext): Promise<boolean>;
  /** Paginated query scoped to accessible client IDs. Null = unrestricted. */
  findPagedScoped(
    page: number,
    pageSize: number,
    clientIds: string[] | null,
    tx?: TransactionContext,
  ): Promise<PagedResult<Client>>;
  /** Search clients by name/identifier with optional status filter. */
  search(
    query: string,
    status: string | undefined,
    limit: number,
    clientIds: string[] | null,
    tx?: TransactionContext,
  ): Promise<Client[]>;
  /** List clients with optional status filter. */
  findByStatus(
    status: string,
    clientIds: string[] | null,
    tx?: TransactionContext,
  ): Promise<Client[]>;
}

/**
 * Create a Client repository.
 */
export function createClientRepository(defaultDb: AnyDb): ClientRepository {
  const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

  return {
    async findById(id: string, tx?: TransactionContext): Promise<Client | undefined> {
      const [record] = await db(tx).select().from(clients).where(eq(clients.id, id)).limit(1);

      if (!record) return undefined;

      return recordToClient(record);
    },

    async findByIdentifier(
      identifier: string,
      tx?: TransactionContext,
    ): Promise<Client | undefined> {
      const [record] = await db(tx)
        .select()
        .from(clients)
        .where(eq(clients.identifier, identifier.toLowerCase()))
        .limit(1);

      if (!record) return undefined;

      return recordToClient(record);
    },

    async findAll(tx?: TransactionContext): Promise<Client[]> {
      const records = await db(tx).select().from(clients);
      return records.map(recordToClient);
    },

    async findPaged(
      page: number,
      pageSize: number,
      tx?: TransactionContext,
    ): Promise<PagedResult<Client>> {
      return this.findPagedScoped(page, pageSize, null, tx);
    },

    async findPagedScoped(
      page: number,
      pageSize: number,
      clientIds: string[] | null,
      tx?: TransactionContext,
    ): Promise<PagedResult<Client>> {
      const whereClause =
        clientIds && clientIds.length > 0
          ? inArray(clients.id, clientIds)
          : clientIds !== null && clientIds?.length === 0
            ? sql`false` // No accessible clients = empty result
            : undefined; // null = unrestricted

      const countQuery = db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(clients);
      const [countResult] = whereClause ? await countQuery.where(whereClause) : await countQuery;
      const totalItems = Number(countResult?.count ?? 0);

      const selectQuery = db(tx)
        .select()
        .from(clients)
        .limit(pageSize)
        .offset(page * pageSize)
        .orderBy(clients.createdAt);
      const records = whereClause ? await selectQuery.where(whereClause) : await selectQuery;

      const items = records.map(recordToClient);
      return createPagedResult(items, page, pageSize, totalItems);
    },

    async search(
      query: string,
      status: string | undefined,
      limit: number,
      clientIds: string[] | null,
      tx?: TransactionContext,
    ): Promise<Client[]> {
      const conditions = [];
      const searchPattern = `%${query}%`;
      conditions.push(
        or(ilike(clients.name, searchPattern), ilike(clients.identifier, searchPattern))!,
      );
      if (status) conditions.push(eq(clients.status, status));
      if (clientIds !== null) {
        if (clientIds.length === 0) return [];
        conditions.push(inArray(clients.id, clientIds));
      }

      const records = await db(tx)
        .select()
        .from(clients)
        .where(and(...conditions))
        .limit(limit)
        .orderBy(clients.name);

      return records.map(recordToClient);
    },

    async findByStatus(
      status: string,
      clientIds: string[] | null,
      tx?: TransactionContext,
    ): Promise<Client[]> {
      const conditions = [eq(clients.status, status)];
      if (clientIds !== null) {
        if (clientIds.length === 0) return [];
        conditions.push(inArray(clients.id, clientIds));
      }

      const records = await db(tx)
        .select()
        .from(clients)
        .where(and(...conditions))
        .orderBy(clients.name);

      return records.map(recordToClient);
    },

    async count(tx?: TransactionContext): Promise<number> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(clients);
      return Number(result?.count ?? 0);
    },

    async exists(id: string, tx?: TransactionContext): Promise<boolean> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(clients)
        .where(eq(clients.id, id));
      return Number(result?.count ?? 0) > 0;
    },

    async existsByIdentifier(identifier: string, tx?: TransactionContext): Promise<boolean> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(clients)
        .where(eq(clients.identifier, identifier.toLowerCase()));
      return Number(result?.count ?? 0) > 0;
    },

    async insert(entity: NewClient, tx?: TransactionContext): Promise<Client> {
      const now = new Date();
      const record: NewClientRecord = {
        id: entity.id,
        name: entity.name,
        identifier: entity.identifier,
        status: entity.status,
        statusReason: entity.statusReason,
        statusChangedAt: entity.statusChangedAt,
        notes: notesToJson(entity.notes),
        createdAt: entity.createdAt ?? now,
        updatedAt: entity.updatedAt ?? now,
      };

      await db(tx).insert(clients).values(record);

      return this.findById(entity.id, tx) as Promise<Client>;
    },

    async update(entity: Client, tx?: TransactionContext): Promise<Client> {
      const now = new Date();

      await db(tx)
        .update(clients)
        .set({
          name: entity.name,
          identifier: entity.identifier,
          status: entity.status,
          statusReason: entity.statusReason,
          statusChangedAt: entity.statusChangedAt,
          notes: notesToJson(entity.notes),
          updatedAt: now,
        })
        .where(eq(clients.id, entity.id));

      return this.findById(entity.id, tx) as Promise<Client>;
    },

    async persist(entity: NewClient, tx?: TransactionContext): Promise<Client> {
      const existing = await this.exists(entity.id, tx);
      if (existing) {
        return this.update(entity as Client, tx);
      }
      return this.insert(entity, tx);
    },

    async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
      const exists = await this.exists(id, tx);
      if (!exists) return false;
      await db(tx).delete(clients).where(eq(clients.id, id));
      return true;
    },

    async delete(entity: Client, tx?: TransactionContext): Promise<boolean> {
      return this.deleteById(entity.id, tx);
    },
  };
}

/**
 * Convert a database record to a Client.
 */
function recordToClient(record: ClientRecord): Client {
  return {
    id: record.id,
    name: record.name,
    identifier: record.identifier,
    status: record.status as ClientStatus,
    statusReason: record.statusReason,
    statusChangedAt: record.statusChangedAt,
    notes: jsonToNotes(record.notes),
    createdAt: record.createdAt,
    updatedAt: record.updatedAt,
  };
}

/**
 * Convert notes to JSON format.
 */
function notesToJson(notes: readonly ClientNote[]): ClientNoteJson[] {
  return notes.map((note) => ({
    category: note.category,
    text: note.text,
    addedBy: note.addedBy,
    addedAt: note.addedAt.toISOString(),
  }));
}

/**
 * Convert JSON notes to domain notes.
 */
function jsonToNotes(json: ClientNoteJson[] | null): ClientNote[] {
  if (!json) return [];
  return json.map((n) => ({
    category: n.category,
    text: n.text,
    addedBy: n.addedBy,
    addedAt: new Date(n.addedAt),
  }));
}
