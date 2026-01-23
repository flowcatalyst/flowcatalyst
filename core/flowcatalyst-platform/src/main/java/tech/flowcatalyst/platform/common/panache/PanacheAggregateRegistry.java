package tech.flowcatalyst.platform.common.panache;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authorization.AuthPermission;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.schema.Schema;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.subscription.Subscription;

import java.lang.reflect.Field;
import java.time.Instant;

/**
 * Panache-based registry for persisting aggregates within a transaction.
 *
 * <p>This replaces the JOOQ-based AggregateRegistry with JPA/Panache operations.
 * All operations use the provided EntityManager to participate in the caller's transaction.
 *
 * <p>Note: This is a transitional implementation that will use JPA native queries
 * until all entity mappings are completed. Future phases will use proper JPA entities.
 */
@ApplicationScoped
public class PanacheAggregateRegistry {

    private static final ObjectMapper objectMapper = new ObjectMapper();

    @Inject
    EntityManager em;

    /**
     * Persist an aggregate (insert or update) within the current transaction.
     *
     * @param aggregate The entity to persist
     */
    public void persist(Object aggregate) {
        if (aggregate == null) {
            throw new IllegalArgumentException("Aggregate cannot be null");
        }

        // Set timestamps for mutable classes
        setTimestamps(aggregate);

        Class<?> clazz = aggregate.getClass();

        // For now, delegate to native queries until all JPA entities are migrated
        // This allows gradual migration without breaking existing functionality
        if (clazz == Application.class) {
            persistApplication((Application) aggregate);
        } else if (clazz == ApplicationClientConfig.class) {
            persistApplicationClientConfig((ApplicationClientConfig) aggregate);
        } else if (clazz == EventType.class) {
            persistEventType((EventType) aggregate);
        } else if (clazz == Subscription.class) {
            persistSubscription((Subscription) aggregate);
        } else if (clazz == Schema.class) {
            persistSchema((Schema) aggregate);
        } else if (clazz == Client.class) {
            persistClient((Client) aggregate);
        } else if (clazz == Principal.class) {
            persistPrincipal((Principal) aggregate);
        } else if (clazz == OAuthClient.class) {
            persistOAuthClient((OAuthClient) aggregate);
        } else if (clazz == ServiceAccount.class) {
            persistServiceAccount((ServiceAccount) aggregate);
        } else if (clazz == AuthRole.class) {
            persistAuthRole((AuthRole) aggregate);
        } else if (clazz == AuthPermission.class) {
            persistAuthPermission((AuthPermission) aggregate);
        } else if (clazz == DispatchPool.class) {
            persistDispatchPool((DispatchPool) aggregate);
        } else if (clazz == ClientAccessGrant.class) {
            persistClientAccessGrant((ClientAccessGrant) aggregate);
        } else {
            throw new IllegalArgumentException("Unknown aggregate type: " + clazz.getName() +
                ". Register it in PanacheAggregateRegistry.");
        }
    }

    /**
     * Delete an aggregate within the current transaction.
     *
     * @param aggregate The entity to delete
     */
    public void delete(Object aggregate) {
        if (aggregate == null) {
            throw new IllegalArgumentException("Aggregate cannot be null");
        }

        Class<?> clazz = aggregate.getClass();
        String id = extractId(aggregate);

        // Handle junction tables for ServiceAccount
        if (clazz == ServiceAccount.class) {
            em.createNativeQuery("DELETE FROM service_account_client_ids WHERE service_account_id = :id")
                .setParameter("id", id)
                .executeUpdate();
            em.createNativeQuery("DELETE FROM service_account_roles WHERE service_account_id = :id")
                .setParameter("id", id)
                .executeUpdate();
        }

        // Handle junction tables for Principal
        if (clazz == Principal.class) {
            em.createNativeQuery("DELETE FROM principal_roles WHERE principal_id = :id")
                .setParameter("id", id)
                .executeUpdate();
            em.createNativeQuery("DELETE FROM principal_managed_applications WHERE principal_id = :id")
                .setParameter("id", id)
                .executeUpdate();
        }

        // Handle junction tables for OAuthClient
        if (clazz == OAuthClient.class) {
            em.createNativeQuery("DELETE FROM oauth_client_redirect_uris WHERE oauth_client_id = :id")
                .setParameter("id", id)
                .executeUpdate();
            em.createNativeQuery("DELETE FROM oauth_client_allowed_origins WHERE oauth_client_id = :id")
                .setParameter("id", id)
                .executeUpdate();
            em.createNativeQuery("DELETE FROM oauth_client_grant_types WHERE oauth_client_id = :id")
                .setParameter("id", id)
                .executeUpdate();
            em.createNativeQuery("DELETE FROM oauth_client_application_ids WHERE oauth_client_id = :id")
                .setParameter("id", id)
                .executeUpdate();
        }

        String tableName = getTableName(clazz);
        if (tableName != null) {
            em.createNativeQuery("DELETE FROM " + tableName + " WHERE id = :id")
                .setParameter("id", id)
                .executeUpdate();
        } else {
            throw new IllegalArgumentException("Unknown aggregate type for delete: " + clazz.getName());
        }
    }

    // ========================================================================
    // Entity-Specific Persist Methods (using native queries for now)
    // ========================================================================

    private void persistApplication(Application app) {
        String sql = """
            INSERT INTO applications (id, code, name, description, type, default_base_url,
                service_account_id, active, icon_url, website, logo, logo_mime_type, created_at, updated_at)
            VALUES (:id, :code, :name, :description, :type, :defaultBaseUrl,
                :serviceAccountId, :active, :iconUrl, :website, :logo, :logoMimeType, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code, name = EXCLUDED.name, description = EXCLUDED.description,
                type = EXCLUDED.type, default_base_url = EXCLUDED.default_base_url,
                service_account_id = EXCLUDED.service_account_id, active = EXCLUDED.active,
                icon_url = EXCLUDED.icon_url, website = EXCLUDED.website, logo = EXCLUDED.logo,
                logo_mime_type = EXCLUDED.logo_mime_type, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", app.id)
            .setParameter("code", app.code)
            .setParameter("name", app.name)
            .setParameter("description", app.description)
            .setParameter("type", app.type != null ? app.type.name() : "APPLICATION")
            .setParameter("defaultBaseUrl", app.defaultBaseUrl)
            .setParameter("serviceAccountId", app.serviceAccountId)
            .setParameter("active", app.active)
            .setParameter("iconUrl", app.iconUrl)
            .setParameter("website", app.website)
            .setParameter("logo", app.logo)
            .setParameter("logoMimeType", app.logoMimeType)
            .setParameter("createdAt", app.createdAt)
            .setParameter("updatedAt", app.updatedAt)
            .executeUpdate();
    }

    private void persistApplicationClientConfig(ApplicationClientConfig config) {
        String sql = """
            INSERT INTO application_client_configs (id, application_id, client_id, enabled,
                base_url_override, website_override, config_json, created_at, updated_at)
            VALUES (:id, :applicationId, :clientId, :enabled, :baseUrlOverride,
                :websiteOverride, CAST(:configJson AS jsonb), :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                application_id = EXCLUDED.application_id, client_id = EXCLUDED.client_id,
                enabled = EXCLUDED.enabled, base_url_override = EXCLUDED.base_url_override,
                website_override = EXCLUDED.website_override, config_json = EXCLUDED.config_json,
                updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", config.id)
            .setParameter("applicationId", config.applicationId)
            .setParameter("clientId", config.clientId)
            .setParameter("enabled", config.enabled)
            .setParameter("baseUrlOverride", config.baseUrlOverride)
            .setParameter("websiteOverride", config.websiteOverride)
            .setParameter("configJson", toJson(config.configJson))
            .setParameter("createdAt", config.createdAt)
            .setParameter("updatedAt", config.updatedAt)
            .executeUpdate();
    }

    private void persistEventType(EventType eventType) {
        String sql = """
            INSERT INTO event_types (id, code, name, description, spec_versions, status, created_at, updated_at)
            VALUES (:id, :code, :name, :description, CAST(:specVersions AS jsonb), :status, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code, name = EXCLUDED.name, description = EXCLUDED.description,
                spec_versions = EXCLUDED.spec_versions, status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", eventType.id())
            .setParameter("code", eventType.code())
            .setParameter("name", eventType.name())
            .setParameter("description", eventType.description())
            .setParameter("specVersions", toJson(eventType.specVersions()))
            .setParameter("status", eventType.status() != null ? eventType.status().name() : "CURRENT")
            .setParameter("createdAt", eventType.createdAt())
            .setParameter("updatedAt", eventType.updatedAt())
            .executeUpdate();
    }

    private void persistSubscription(Subscription sub) {
        String sql = """
            INSERT INTO subscriptions (id, code, name, description, client_id, client_identifier,
                event_types, target, queue, custom_config, source, status, max_age_seconds,
                dispatch_pool_id, dispatch_pool_code, delay_seconds, sequence, mode,
                timeout_seconds, max_retries, service_account_id, data_only, created_at, updated_at)
            VALUES (:id, :code, :name, :description, :clientId, :clientIdentifier,
                CAST(:eventTypes AS jsonb), :target, :queue, CAST(:customConfig AS jsonb), :source, :status, :maxAgeSeconds,
                :dispatchPoolId, :dispatchPoolCode, :delaySeconds, :sequence, :mode,
                :timeoutSeconds, :maxRetries, :serviceAccountId, :dataOnly, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code, name = EXCLUDED.name, description = EXCLUDED.description,
                client_id = EXCLUDED.client_id, client_identifier = EXCLUDED.client_identifier,
                event_types = EXCLUDED.event_types, target = EXCLUDED.target, queue = EXCLUDED.queue,
                custom_config = EXCLUDED.custom_config, source = EXCLUDED.source, status = EXCLUDED.status,
                max_age_seconds = EXCLUDED.max_age_seconds, dispatch_pool_id = EXCLUDED.dispatch_pool_id,
                dispatch_pool_code = EXCLUDED.dispatch_pool_code, delay_seconds = EXCLUDED.delay_seconds,
                sequence = EXCLUDED.sequence, mode = EXCLUDED.mode, timeout_seconds = EXCLUDED.timeout_seconds,
                max_retries = EXCLUDED.max_retries, service_account_id = EXCLUDED.service_account_id,
                data_only = EXCLUDED.data_only, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", sub.id())
            .setParameter("code", sub.code())
            .setParameter("name", sub.name())
            .setParameter("description", sub.description())
            .setParameter("clientId", sub.clientId())
            .setParameter("clientIdentifier", sub.clientIdentifier())
            .setParameter("eventTypes", toJson(sub.eventTypes()))
            .setParameter("target", sub.target())
            .setParameter("queue", sub.queue())
            .setParameter("customConfig", toJson(sub.customConfig()))
            .setParameter("source", sub.source() != null ? sub.source().name() : "API")
            .setParameter("status", sub.status() != null ? sub.status().name() : "ACTIVE")
            .setParameter("maxAgeSeconds", sub.maxAgeSeconds())
            .setParameter("dispatchPoolId", sub.dispatchPoolId())
            .setParameter("dispatchPoolCode", sub.dispatchPoolCode())
            .setParameter("delaySeconds", sub.delaySeconds())
            .setParameter("sequence", sub.sequence())
            .setParameter("mode", sub.mode() != null ? sub.mode().name() : "IMMEDIATE")
            .setParameter("timeoutSeconds", sub.timeoutSeconds())
            .setParameter("maxRetries", sub.maxRetries())
            .setParameter("serviceAccountId", sub.serviceAccountId())
            .setParameter("dataOnly", sub.dataOnly())
            .setParameter("createdAt", sub.createdAt())
            .setParameter("updatedAt", sub.updatedAt())
            .executeUpdate();
    }

    private void persistSchema(Schema schema) {
        String sql = """
            INSERT INTO schemas (id, name, description, mime_type, schema_type, content,
                event_type_id, version, created_at, updated_at)
            VALUES (:id, :name, :description, :mimeType, :schemaType, :content,
                :eventTypeId, :version, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                name = EXCLUDED.name, description = EXCLUDED.description, mime_type = EXCLUDED.mime_type,
                schema_type = EXCLUDED.schema_type, content = EXCLUDED.content,
                event_type_id = EXCLUDED.event_type_id, version = EXCLUDED.version, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", schema.id())
            .setParameter("name", schema.name())
            .setParameter("description", schema.description())
            .setParameter("mimeType", schema.mimeType())
            .setParameter("schemaType", schema.schemaType() != null ? schema.schemaType().name() : null)
            .setParameter("content", schema.content())
            .setParameter("eventTypeId", schema.eventTypeId())
            .setParameter("version", schema.version())
            .setParameter("createdAt", schema.createdAt())
            .setParameter("updatedAt", schema.updatedAt())
            .executeUpdate();
    }

    private void persistClient(Client client) {
        String sql = """
            INSERT INTO clients (id, name, identifier, status, status_reason, status_changed_at,
                notes, created_at, updated_at)
            VALUES (:id, :name, :identifier, :status, :statusReason, :statusChangedAt,
                CAST(:notes AS jsonb), :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                name = EXCLUDED.name, identifier = EXCLUDED.identifier, status = EXCLUDED.status,
                status_reason = EXCLUDED.status_reason, status_changed_at = EXCLUDED.status_changed_at,
                notes = EXCLUDED.notes, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", client.id)
            .setParameter("name", client.name)
            .setParameter("identifier", client.identifier)
            .setParameter("status", client.status != null ? client.status.name() : "ACTIVE")
            .setParameter("statusReason", client.statusReason)
            .setParameter("statusChangedAt", client.statusChangedAt)
            .setParameter("notes", toJson(client.notes))
            .setParameter("createdAt", client.createdAt)
            .setParameter("updatedAt", client.updatedAt)
            .executeUpdate();
    }

    private void persistPrincipal(Principal principal) {
        var ui = principal.userIdentity;
        // Note: roles column was normalized to principal_roles table in V8/V12 migrations
        // Note: service_account_id added in V22 - links to ServiceAccount entity for SERVICE type
        // Note: application_id and service_account columns are deprecated (set to NULL)
        String sql = """
            INSERT INTO principals (id, type, scope, client_id, application_id, name, active,
                email, email_domain, idp_type, external_idp_id, password_hash, last_login_at,
                service_account_id, service_account, created_at, updated_at)
            VALUES (:id, :type, :scope, :clientId, NULL, :name, :active,
                :email, :emailDomain, :idpType, :externalIdpId, :passwordHash, :lastLoginAt,
                :serviceAccountId, NULL, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                type = EXCLUDED.type, scope = EXCLUDED.scope, client_id = EXCLUDED.client_id,
                name = EXCLUDED.name, active = EXCLUDED.active,
                email = EXCLUDED.email, email_domain = EXCLUDED.email_domain, idp_type = EXCLUDED.idp_type,
                external_idp_id = EXCLUDED.external_idp_id, password_hash = EXCLUDED.password_hash,
                last_login_at = EXCLUDED.last_login_at, service_account_id = EXCLUDED.service_account_id,
                updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", principal.id)
            .setParameter("type", principal.type != null ? principal.type.name() : "USER")
            .setParameter("scope", principal.scope != null ? principal.scope.name() : null)
            .setParameter("clientId", principal.clientId)
            .setParameter("name", principal.name)
            .setParameter("active", principal.active)
            .setParameter("email", ui != null ? ui.email : null)
            .setParameter("emailDomain", ui != null ? ui.emailDomain : null)
            .setParameter("idpType", ui != null && ui.idpType != null ? ui.idpType.name() : null)
            .setParameter("externalIdpId", ui != null ? ui.externalIdpId : null)
            .setParameter("passwordHash", ui != null ? ui.passwordHash : null)
            .setParameter("lastLoginAt", ui != null ? ui.lastLoginAt : null)
            .setParameter("serviceAccountId", principal.serviceAccountId)
            .setParameter("createdAt", principal.createdAt)
            .setParameter("updatedAt", principal.updatedAt)
            .executeUpdate();

        // Save roles to normalized principal_roles table
        savePrincipalRoles(principal);
    }

    private void savePrincipalRoles(Principal principal) {
        // Delete existing roles
        em.createNativeQuery("DELETE FROM principal_roles WHERE principal_id = :id")
            .setParameter("id", principal.id)
            .executeUpdate();

        // Insert current roles
        if (principal.roles != null) {
            for (var role : principal.roles) {
                em.createNativeQuery("""
                    INSERT INTO principal_roles (principal_id, role_name, assignment_source, assigned_at)
                    VALUES (:principalId, :roleName, :assignmentSource, :assignedAt)
                    """)
                    .setParameter("principalId", principal.id)
                    .setParameter("roleName", role.roleName)
                    .setParameter("assignmentSource", role.assignmentSource)
                    .setParameter("assignedAt", role.assignedAt != null ? role.assignedAt : Instant.now())
                    .executeUpdate();
            }
        }
    }

    private void persistOAuthClient(OAuthClient client) {
        // Insert/update main oauth_clients table (without array columns - now normalized)
        String sql = """
            INSERT INTO oauth_clients (id, client_id, client_name, client_type, client_secret_ref,
                default_scopes, pkce_required, service_account_principal_id, active, created_at, updated_at)
            VALUES (:id, :oauthClientId, :clientName, :clientType, :clientSecretRef,
                :defaultScopes, :pkceRequired, :serviceAccountPrincipalId, :active, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                client_id = EXCLUDED.client_id, client_name = EXCLUDED.client_name,
                client_type = EXCLUDED.client_type, client_secret_ref = EXCLUDED.client_secret_ref,
                default_scopes = EXCLUDED.default_scopes, pkce_required = EXCLUDED.pkce_required,
                service_account_principal_id = EXCLUDED.service_account_principal_id,
                active = EXCLUDED.active, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", client.id)
            .setParameter("oauthClientId", client.clientId)
            .setParameter("clientName", client.clientName)
            .setParameter("clientType", client.clientType != null ? client.clientType.name() : "PUBLIC")
            .setParameter("clientSecretRef", client.clientSecretRef)
            .setParameter("defaultScopes", client.defaultScopes)
            .setParameter("pkceRequired", client.pkceRequired)
            .setParameter("serviceAccountPrincipalId", client.serviceAccountPrincipalId)
            .setParameter("active", client.active)
            .setParameter("createdAt", client.createdAt)
            .setParameter("updatedAt", client.updatedAt)
            .executeUpdate();

        // Save to normalized collection tables
        saveOAuthClientCollections(client);
    }

    private void saveOAuthClientCollections(OAuthClient client) {
        // Delete existing entries
        em.createNativeQuery("DELETE FROM oauth_client_redirect_uris WHERE oauth_client_id = :id")
            .setParameter("id", client.id).executeUpdate();
        em.createNativeQuery("DELETE FROM oauth_client_allowed_origins WHERE oauth_client_id = :id")
            .setParameter("id", client.id).executeUpdate();
        em.createNativeQuery("DELETE FROM oauth_client_grant_types WHERE oauth_client_id = :id")
            .setParameter("id", client.id).executeUpdate();
        em.createNativeQuery("DELETE FROM oauth_client_application_ids WHERE oauth_client_id = :id")
            .setParameter("id", client.id).executeUpdate();

        // Insert redirect URIs
        if (client.redirectUris != null) {
            for (String uri : client.redirectUris) {
                em.createNativeQuery("INSERT INTO oauth_client_redirect_uris (oauth_client_id, redirect_uri) VALUES (:id, :uri)")
                    .setParameter("id", client.id).setParameter("uri", uri).executeUpdate();
            }
        }

        // Insert allowed origins
        if (client.allowedOrigins != null) {
            for (String origin : client.allowedOrigins) {
                em.createNativeQuery("INSERT INTO oauth_client_allowed_origins (oauth_client_id, allowed_origin) VALUES (:id, :origin)")
                    .setParameter("id", client.id).setParameter("origin", origin).executeUpdate();
            }
        }

        // Insert grant types
        if (client.grantTypes != null) {
            for (String grantType : client.grantTypes) {
                em.createNativeQuery("INSERT INTO oauth_client_grant_types (oauth_client_id, grant_type) VALUES (:id, :grantType)")
                    .setParameter("id", client.id).setParameter("grantType", grantType).executeUpdate();
            }
        }

        // Insert application IDs
        if (client.applicationIds != null) {
            for (String appId : client.applicationIds) {
                em.createNativeQuery("INSERT INTO oauth_client_application_ids (oauth_client_id, application_id) VALUES (:id, :appId)")
                    .setParameter("id", client.id).setParameter("appId", appId).executeUpdate();
            }
        }
    }

    private void persistServiceAccount(ServiceAccount sa) {
        var wc = sa.webhookCredentials;
        // Note: client_ids and roles columns were normalized to junction tables in V12 migration
        String sql = """
            INSERT INTO service_accounts (id, code, name, description, application_id,
                active, wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm,
                wh_credentials_created_at, wh_credentials_regenerated_at, last_used_at, created_at, updated_at)
            VALUES (:id, :code, :name, :description, :applicationId,
                :active, :whAuthType, :whAuthTokenRef, :whSigningSecretRef, :whSigningAlgorithm,
                :whCredentialsCreatedAt, :whCredentialsRegeneratedAt, :lastUsedAt, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code, name = EXCLUDED.name, description = EXCLUDED.description,
                application_id = EXCLUDED.application_id,
                active = EXCLUDED.active, wh_auth_type = EXCLUDED.wh_auth_type,
                wh_auth_token_ref = EXCLUDED.wh_auth_token_ref, wh_signing_secret_ref = EXCLUDED.wh_signing_secret_ref,
                wh_signing_algorithm = EXCLUDED.wh_signing_algorithm, wh_credentials_created_at = EXCLUDED.wh_credentials_created_at,
                wh_credentials_regenerated_at = EXCLUDED.wh_credentials_regenerated_at,
                last_used_at = EXCLUDED.last_used_at, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", sa.id)
            .setParameter("code", sa.code)
            .setParameter("name", sa.name)
            .setParameter("description", sa.description)
            .setParameter("applicationId", sa.applicationId)
            .setParameter("active", sa.active)
            .setParameter("whAuthType", wc != null && wc.authType != null ? wc.authType.name() : null)
            .setParameter("whAuthTokenRef", wc != null ? wc.authTokenRef : null)
            .setParameter("whSigningSecretRef", wc != null ? wc.signingSecretRef : null)
            .setParameter("whSigningAlgorithm", wc != null && wc.signingAlgorithm != null ? wc.signingAlgorithm.name() : null)
            .setParameter("whCredentialsCreatedAt", wc != null ? wc.createdAt : null)
            .setParameter("whCredentialsRegeneratedAt", wc != null ? wc.regeneratedAt : null)
            .setParameter("lastUsedAt", sa.lastUsedAt)
            .setParameter("createdAt", sa.createdAt)
            .setParameter("updatedAt", sa.updatedAt)
            .executeUpdate();

        // Save client IDs to normalized junction table
        saveServiceAccountClientIds(sa);

        // Save roles to normalized junction table
        saveServiceAccountRoles(sa);
    }

    private void saveServiceAccountClientIds(ServiceAccount sa) {
        // Delete existing client ID associations
        em.createNativeQuery("DELETE FROM service_account_client_ids WHERE service_account_id = :id")
            .setParameter("id", sa.id)
            .executeUpdate();

        // Insert current client IDs
        if (sa.clientIds != null) {
            for (String clientId : sa.clientIds) {
                em.createNativeQuery("""
                    INSERT INTO service_account_client_ids (service_account_id, client_id)
                    VALUES (:serviceAccountId, :clientId)
                    """)
                    .setParameter("serviceAccountId", sa.id)
                    .setParameter("clientId", clientId)
                    .executeUpdate();
            }
        }
    }

    private void saveServiceAccountRoles(ServiceAccount sa) {
        // Delete existing role associations
        em.createNativeQuery("DELETE FROM service_account_roles WHERE service_account_id = :id")
            .setParameter("id", sa.id)
            .executeUpdate();

        // Insert current roles
        if (sa.roles != null) {
            for (var role : sa.roles) {
                em.createNativeQuery("""
                    INSERT INTO service_account_roles (service_account_id, role_name, assignment_source, assigned_at)
                    VALUES (:serviceAccountId, :roleName, :assignmentSource, :assignedAt)
                    """)
                    .setParameter("serviceAccountId", sa.id)
                    .setParameter("roleName", role.roleName)
                    .setParameter("assignmentSource", role.assignmentSource)
                    .setParameter("assignedAt", role.assignedAt != null ? role.assignedAt : Instant.now())
                    .executeUpdate();
            }
        }
    }

    private void persistAuthRole(AuthRole role) {
        String sql = """
            INSERT INTO auth_roles (id, application_id, application_code, name, display_name,
                description, permissions, source, client_managed, created_at, updated_at)
            VALUES (:id, :applicationId, :applicationCode, :name, :displayName,
                :description, :permissions, :source, :clientManaged, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                application_id = EXCLUDED.application_id, application_code = EXCLUDED.application_code,
                name = EXCLUDED.name, display_name = EXCLUDED.display_name, description = EXCLUDED.description,
                permissions = EXCLUDED.permissions, source = EXCLUDED.source,
                client_managed = EXCLUDED.client_managed, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", role.id)
            .setParameter("applicationId", role.applicationId)
            .setParameter("applicationCode", role.applicationCode)
            .setParameter("name", role.name)
            .setParameter("displayName", role.displayName)
            .setParameter("description", role.description)
            .setParameter("permissions", role.permissions != null ? role.permissions.toArray(new String[0]) : new String[0])
            .setParameter("source", role.source != null ? role.source.name() : "DATABASE")
            .setParameter("clientManaged", role.clientManaged)
            .setParameter("createdAt", role.createdAt)
            .setParameter("updatedAt", role.updatedAt)
            .executeUpdate();
    }

    private void persistAuthPermission(AuthPermission perm) {
        String sql = """
            INSERT INTO auth_permissions (id, application_id, name, display_name, description, source, created_at)
            VALUES (:id, :applicationId, :name, :displayName, :description, :source, :createdAt)
            ON CONFLICT (id) DO UPDATE SET
                application_id = EXCLUDED.application_id, name = EXCLUDED.name,
                display_name = EXCLUDED.display_name, description = EXCLUDED.description, source = EXCLUDED.source
            """;
        em.createNativeQuery(sql)
            .setParameter("id", perm.id)
            .setParameter("applicationId", perm.applicationId)
            .setParameter("name", perm.name)
            .setParameter("displayName", perm.displayName)
            .setParameter("description", perm.description)
            .setParameter("source", perm.source != null ? perm.source.name() : "SDK")
            .setParameter("createdAt", perm.createdAt)
            .executeUpdate();
    }

    private void persistDispatchPool(DispatchPool pool) {
        String sql = """
            INSERT INTO dispatch_pools (id, code, name, description, rate_limit, concurrency,
                client_id, client_identifier, status, created_at, updated_at)
            VALUES (:id, :code, :name, :description, :rateLimit, :concurrency,
                :clientId, :clientIdentifier, :status, :createdAt, :updatedAt)
            ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code, name = EXCLUDED.name, description = EXCLUDED.description,
                rate_limit = EXCLUDED.rate_limit, concurrency = EXCLUDED.concurrency,
                client_id = EXCLUDED.client_id, client_identifier = EXCLUDED.client_identifier,
                status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", pool.id())
            .setParameter("code", pool.code())
            .setParameter("name", pool.name())
            .setParameter("description", pool.description())
            .setParameter("rateLimit", pool.rateLimit())
            .setParameter("concurrency", pool.concurrency())
            .setParameter("clientId", pool.clientId())
            .setParameter("clientIdentifier", pool.clientIdentifier())
            .setParameter("status", pool.status() != null ? pool.status().name() : "ACTIVE")
            .setParameter("createdAt", pool.createdAt())
            .setParameter("updatedAt", pool.updatedAt())
            .executeUpdate();
    }

    private void persistClientAccessGrant(ClientAccessGrant grant) {
        String sql = """
            INSERT INTO client_access_grants (id, principal_id, client_id, granted_at, expires_at)
            VALUES (:id, :principalId, :clientId, :grantedAt, :expiresAt)
            ON CONFLICT (id) DO UPDATE SET
                principal_id = EXCLUDED.principal_id, client_id = EXCLUDED.client_id,
                granted_at = EXCLUDED.granted_at, expires_at = EXCLUDED.expires_at
            """;
        em.createNativeQuery(sql)
            .setParameter("id", grant.id)
            .setParameter("principalId", grant.principalId)
            .setParameter("clientId", grant.clientId)
            .setParameter("grantedAt", grant.grantedAt)
            .setParameter("expiresAt", grant.expiresAt)
            .executeUpdate();
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private String getTableName(Class<?> clazz) {
        if (clazz == Application.class) return "applications";
        if (clazz == ApplicationClientConfig.class) return "application_client_configs";
        if (clazz == EventType.class) return "event_types";
        if (clazz == Subscription.class) return "subscriptions";
        if (clazz == Schema.class) return "schemas";
        if (clazz == Client.class) return "clients";
        if (clazz == Principal.class) return "principals";
        if (clazz == OAuthClient.class) return "oauth_clients";
        if (clazz == ServiceAccount.class) return "service_accounts";
        if (clazz == AuthRole.class) return "auth_roles";
        if (clazz == AuthPermission.class) return "auth_permissions";
        if (clazz == DispatchPool.class) return "dispatch_pools";
        if (clazz == ClientAccessGrant.class) return "client_access_grants";
        return null;
    }

    private void setTimestamps(Object aggregate) {
        if (aggregate.getClass().isRecord()) {
            return;
        }

        try {
            Field updatedAtField = aggregate.getClass().getField("updatedAt");
            if (updatedAtField.getType() == Instant.class) {
                updatedAtField.set(aggregate, Instant.now());
            }
        } catch (NoSuchFieldException | IllegalAccessException ignored) {}

        try {
            Field createdAtField = aggregate.getClass().getField("createdAt");
            if (createdAtField.getType() == Instant.class && createdAtField.get(aggregate) == null) {
                createdAtField.set(aggregate, Instant.now());
            }
        } catch (NoSuchFieldException | IllegalAccessException ignored) {}
    }

    private String extractId(Object aggregate) {
        Class<?> clazz = aggregate.getClass();

        if (clazz.isRecord()) {
            try {
                var method = clazz.getMethod("id");
                return (String) method.invoke(aggregate);
            } catch (Exception e) {
                throw new IllegalArgumentException(
                    "Record must have an id() accessor returning String: " + clazz.getName(), e);
            }
        }

        try {
            Field field = clazz.getField("id");
            return (String) field.get(aggregate);
        } catch (NoSuchFieldException | IllegalAccessException e) {
            throw new IllegalArgumentException(
                "Aggregate must have a public String id field: " + clazz.getName(), e);
        }
    }

    private String toJson(Object obj) {
        if (obj == null) return null;
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (Exception ex) {
            return null;
        }
    }
}
