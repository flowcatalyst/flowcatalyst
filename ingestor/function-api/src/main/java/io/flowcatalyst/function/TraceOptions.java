package io.flowcatalyst.function;

import org.jspecify.annotations.Nullable;

/// How one [FunctionContext#trace] record is written.
///
/// @param level     `debug`, `info`, `warn` or `error`; `null` or anything else is `info`
/// @param reference a business key for this record only (cut to 200 characters); `null` leaves the invocation's own
///                  ([FunctionContext#setReference]) on it
public record TraceOptions(@Nullable String level, @Nullable String reference) {
}
