package io.flowcatalyst.sdk.sync;

import java.util.List;

/**
 * Aggregate result of syncing a full {@link Definitions.DefinitionSet}. Each
 * category is either {@link Category.Synced} (mirroring the platform's
 * {@code SyncResultResponse}), the {@link Category.Skipped} sentinel when the
 * category wasn't part of the submitted set, or {@link Category.Failed} when
 * a LOCAL check (a duplicate code within one sync scope) or a caught HTTP
 * failure stopped it — currently only {@link #connections} and {@link
 * #subscriptions} report {@code Failed}; every other category still lets its
 * HTTP exception propagate, unchanged from before. A {@code Failed}
 * anywhere in the result means {@link DefinitionSynchronizer#sync},
 * {@link DefinitionSynchronizer#syncAll} or {@link
 * DefinitionSynchronizer#syncGrouped} threw {@link DefinitionSyncException}
 * rather than returning this record directly — see that exception for how to
 * read a partial outcome.
 */
public record SyncResult(
        String applicationCode,
        Category roles,
        Category eventTypes,
        /*
         * Connections sync BEFORE subscriptions (a subscription's
         * connectionCode must resolve in the same run) — positioned here to
         * match.
         */
        Category connections,
        Category subscriptions,
        Category dispatchPools,
        Category principals,
        Category processes,
        Category scheduledJobs,
        /*
         * OpenAPI sync is a single-document upload: on success syncedCodes
         * carries [version]; created/updated reflect newly-published vs
         * replaced (both zero on a byte-identical re-sync).
         */
        Category openapi,
        /*
         * Documentation pages (POST /api/applications/{app}/docs/sync). The
         * payload is the application's whole page set: pages not sent are
         * deleted (removeUnlisted does not apply). Skipped when the set
         * declares no pages.
         */
        Category docs,
        /*
         * Push topic rules + ticket keys (POST /api/applications/{app}/push/sync): a
         * Category.PushSynced with the gateway's counts, Skipped when the set declares none, or
         * Failed with the gateway's refusal. Declarative, like docs: removeUnlisted does not apply.
         */
        Category push) {

    /** The shape without {@code push} (reported {@link Category#SKIPPED}). */
    public SyncResult(
            String applicationCode,
            Category roles,
            Category eventTypes,
            Category connections,
            Category subscriptions,
            Category dispatchPools,
            Category principals,
            Category processes,
            Category scheduledJobs,
            Category openapi,
            Category docs) {
        this(applicationCode, roles, eventTypes, connections, subscriptions, dispatchPools,
                principals, processes, scheduledJobs, openapi, docs, Category.SKIPPED);
    }

    /** The 0.0.12 shape, without {@code docs} (reported {@link Category#SKIPPED}). */
    public SyncResult(
            String applicationCode,
            Category roles,
            Category eventTypes,
            Category connections,
            Category subscriptions,
            Category dispatchPools,
            Category principals,
            Category processes,
            Category scheduledJobs,
            Category openapi) {
        this(applicationCode, roles, eventTypes, connections, subscriptions, dispatchPools,
                principals, processes, scheduledJobs, openapi, Category.SKIPPED, Category.SKIPPED);
    }

    /** Per-category outcome. */
    public sealed interface Category {

        /**
         * The platform's {@code SyncResultResponse}.
         *
         * @param skippedRoles principal sync only: role names that are not roles, so were not
         *     assigned to anyone (lower-cased, each once, in payload order); the rest of the sync
         *     applied. Empty when none (the platform omits it), and always empty for every other
         *     category
         * @param passwordHashIgnored principal sync only: emails whose {@code passwordHash} was
         *     not applied because the user already exists (the platform stores a hash only on a
         *     user the sync creates). Empty when none
         */
        record Synced(
                String applicationCode,
                int created,
                int updated,
                int deleted,
                List<String> syncedCodes,
                List<String> skippedRoles,
                List<String> passwordHashIgnored)
                implements Category {

            public Synced {
                syncedCodes = syncedCodes == null ? List.of() : List.copyOf(syncedCodes);
                skippedRoles = skippedRoles == null ? List.of() : List.copyOf(skippedRoles);
                passwordHashIgnored =
                        passwordHashIgnored == null ? List.of() : List.copyOf(passwordHashIgnored);
            }

            /** The 0.0.12 shape: nothing skipped, no hash ignored. */
            public Synced(String applicationCode, int created, int updated, int deleted,
                    List<String> syncedCodes) {
                this(applicationCode, created, updated, deleted, syncedCodes, List.of(), List.of());
            }
        }

        /**
         * The push gateway's {@code PushSyncResponse}: the counts of the rules and of the keys,
         * and the patterns and key ids it holds after the sync.
         */
        record PushSynced(
                String applicationCode,
                Counts rules,
                Counts keys,
                List<String> syncedPatterns,
                List<String> syncedKids)
                implements Category {

            /** Created, updated and deleted rows of one of the two halves. */
            public record Counts(int created, int updated, int deleted) {}

            public PushSynced {
                syncedPatterns = syncedPatterns == null ? List.of() : List.copyOf(syncedPatterns);
                syncedKids = syncedKids == null ? List.of() : List.copyOf(syncedKids);
            }

            public int created() {
                return rules.created() + keys.created();
            }

            public int updated() {
                return rules.updated() + keys.updated();
            }

            public int deleted() {
                return rules.deleted() + keys.deleted();
            }
        }

        record Skipped() implements Category {}

        /**
         * A category that could not be synced: either a local validation
         * failure (two definitions in the same scope sharing a code) that
         * meant nothing was sent, or an HTTP failure caught so sibling
         * scopes could still be attempted (a connection sync failing skips
         * just that scope's subscriptions). {@code created}/{@code
         * updated}/{@code deleted}/{@code syncedCodes} reflect any OTHER
         * scope that synced successfully before or alongside the failure —
         * they are not necessarily all zero.
         */
        record Failed(
                int created,
                int updated,
                int deleted,
                List<String> syncedCodes,
                String error)
                implements Category {}

        Category SKIPPED = new Skipped();

        default boolean isSynced() {
            return this instanceof Synced;
        }

        default boolean isFailed() {
            return this instanceof Failed;
        }
    }
}
