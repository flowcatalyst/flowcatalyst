package tech.flowcatalyst.platform.application.mapper;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.entity.ApplicationClientConfigEntity;

import java.util.HashMap;
import java.util.Map;

/**
 * Mapper for converting between ApplicationClientConfig domain model and JPA entity.
 */
public final class ApplicationClientConfigMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper();

    private ApplicationClientConfigMapper() {
    }

    public static ApplicationClientConfig toDomain(ApplicationClientConfigEntity entity) {
        if (entity == null) {
            return null;
        }

        ApplicationClientConfig domain = new ApplicationClientConfig();
        domain.id = entity.id;
        domain.applicationId = entity.applicationId;
        domain.clientId = entity.clientId;
        domain.enabled = entity.enabled;
        domain.baseUrlOverride = entity.baseUrlOverride;
        domain.websiteOverride = entity.websiteOverride;
        domain.configJson = parseConfigJson(entity.configJson);
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static ApplicationClientConfigEntity toEntity(ApplicationClientConfig domain) {
        if (domain == null) {
            return null;
        }

        ApplicationClientConfigEntity entity = new ApplicationClientConfigEntity();
        entity.id = domain.id;
        entity.applicationId = domain.applicationId;
        entity.clientId = domain.clientId;
        entity.enabled = domain.enabled;
        entity.baseUrlOverride = domain.baseUrlOverride;
        entity.websiteOverride = domain.websiteOverride;
        entity.configJson = toJson(domain.configJson);
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(ApplicationClientConfigEntity entity, ApplicationClientConfig domain) {
        entity.applicationId = domain.applicationId;
        entity.clientId = domain.clientId;
        entity.enabled = domain.enabled;
        entity.baseUrlOverride = domain.baseUrlOverride;
        entity.websiteOverride = domain.websiteOverride;
        entity.configJson = toJson(domain.configJson);
        entity.updatedAt = domain.updatedAt;
    }

    private static Map<String, Object> parseConfigJson(String json) {
        if (json == null || json.isBlank()) {
            return new HashMap<>();
        }
        try {
            return objectMapper.readValue(json, new TypeReference<Map<String, Object>>() {});
        } catch (Exception e) {
            return new HashMap<>();
        }
    }

    private static String toJson(Object obj) {
        if (obj == null) {
            return null;
        }
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (Exception e) {
            return null;
        }
    }
}
