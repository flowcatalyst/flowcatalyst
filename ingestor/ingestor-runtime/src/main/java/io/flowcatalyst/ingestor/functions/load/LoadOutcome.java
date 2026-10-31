package io.flowcatalyst.ingestor.functions.load;

/// The result of one [JvmFunctionLoader#load] attempt. A refusal is a
/// routine outcome, not an exception — the caller switches exhaustively
/// rather than catching.
public sealed interface LoadOutcome permits Loaded, Refused {
}
