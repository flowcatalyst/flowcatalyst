package io.flowcatalyst.sdk.push;

/** A ticket key, ticket request or minter the push contract does not allow. */
public final class TicketException extends IllegalArgumentException {
    public TicketException(String message) {
        super(message);
    }
}
