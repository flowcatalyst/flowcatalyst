/// Function traces (docs/jvm-function-trace-plan.md): the windows an operator switched on, the per-invocation gate, the
/// records an invocation writes (`http`, `log`, `outbound`, and the function's own `trace`), the values replaced in
/// them, and the shipper that carries them to the platform.
@NullMarked
package io.flowcatalyst.ingestor.functions.trace;

import org.jspecify.annotations.NullMarked;
