package tech.flowcatalyst.eventtype;

/**
 * Type of schema definition.
 */
public enum SchemaType {
    /**
     * JSON Schema (draft-07 or later).
     */
    JSON_SCHEMA,

    /**
     * Protocol Buffers schema.
     */
    PROTO,

    /**
     * XML Schema Definition.
     */
    XSD
}
