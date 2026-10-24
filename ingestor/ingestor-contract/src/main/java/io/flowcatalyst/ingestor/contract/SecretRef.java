package io.flowcatalyst.ingestor.contract;

/**
 * The name of a secret. The contract never carries a secret's value; the runtime resolves the
 * name against its own secret stores, for the client of the ingestion that names it. The name
 * never says whose secret it is.
 *
 * @param name the secret's name, such as {@code sftp-password}: lowercase letters and digits in
 *     groups joined by single hyphens. {@code files:sftp-password} names it in the runtime's store
 *     {@code files} instead of its default one
 */
public record SecretRef(String name) {}
