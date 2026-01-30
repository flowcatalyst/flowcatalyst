package tech.flowcatalyst.platform.authentication.domain.operations.deletemapping;

/**
 * Command to delete an Email Domain Mapping.
 *
 * @param emailDomainMappingId ID of the mapping to delete
 */
public record DeleteEmailDomainMappingCommand(
    String emailDomainMappingId
) {}
