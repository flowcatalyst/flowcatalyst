package io.flowcatalyst.field.rn

import io.flowcatalyst.field.FieldAuthException
import io.flowcatalyst.field.FcFieldException
import io.flowcatalyst.field.FieldSession
import io.flowcatalyst.field.FieldConfig
import kotlinx.coroutines.CancellationException

/** What a promise rejection carries. `message` never contains a PIN or a token. */
internal data class Rejection(
    val code: String,
    val message: String,
    val status: Int? = null,
    val retryAfterSeconds: Int? = null,
    val violations: List<String> = emptyList(),
)

internal object NativeErrors {
    /** FieldAuthException keeps its code; key/store FcFieldException keeps its code; a config clash is CONFIG_MISMATCH. */
    fun classify(t: Throwable): Rejection = when (t) {
        is FieldAuthException -> Rejection(t.code, t.message ?: t.code, t.status, t.retryAfterSeconds, t.violations)
        is FcFieldException -> Rejection(t.code, t.message ?: t.code)
        is IllegalStateException ->
            if (t.message?.contains("different FieldConfig") == true) Rejection("CONFIG_MISMATCH", "FieldSession is already configured with a different config")
            else Rejection("INTERNAL", "unexpected native error")
        // Lost keys / OEM Keystore faults must never crash the app.
        else -> Rejection("INTERNAL", "unexpected native error")
    }

    fun isCancellation(t: Throwable): Boolean = t is CancellationException
}
