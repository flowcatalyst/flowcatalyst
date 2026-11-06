package io.flowcatalyst.field.rn

import io.flowcatalyst.field.FieldAuthException
import io.flowcatalyst.field.FcFieldException
import io.flowcatalyst.field.FieldSession
import io.flowcatalyst.field.FieldConfig
import kotlinx.coroutines.CancellationException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NativeErrorsTest {
    @Test fun fieldAuthExceptionKeepsCodeAndExtras() {
        val r = NativeErrors.classify(
            FieldAuthException("WORKER_LOCKED_TEMPORARILY", "locked", status = 423, retryAfterSeconds = 30, violations = listOf("v")),
        )
        assertEquals("WORKER_LOCKED_TEMPORARILY", r.code)
        assertEquals("locked", r.message)
        assertEquals(423, r.status)
        assertEquals(30, r.retryAfterSeconds)
        assertEquals(listOf("v"), r.violations)
    }

    @Test fun lowerCaseDpopCodesPassThrough() {
        assertEquals("use_dpop_nonce", NativeErrors.classify(FieldAuthException("use_dpop_nonce")).code)
        assertEquals("invalid_token", NativeErrors.classify(FieldAuthException("invalid_token")).code)
    }

    @Test fun keyAndStoreErrorsKeepTheirCode() {
        assertEquals("KEY_INVALIDATED", NativeErrors.classify(FcFieldException("KEY_INVALIDATED", "gone")).code)
        assertEquals("BIOMETRIC_CANCELLED", NativeErrors.classify(FcFieldException("BIOMETRIC_CANCELLED", "x")).code)
    }

    @Test fun configClashIsConfigMismatch() {
        val r = NativeErrors.classify(IllegalStateException("FieldSession is already configured with a different FieldConfig"))
        assertEquals("CONFIG_MISMATCH", r.code)
    }

    @Test fun anythingElseIsInternalAndLeaksNothing() {
        val r = NativeErrors.classify(RuntimeException("pin=1234 token=abc"))
        assertEquals("INTERNAL", r.code)
        assertFalse(r.message.contains("1234"))
        assertEquals("INTERNAL", NativeErrors.classify(IllegalStateException("other")).code)
    }

    @Test fun cancellationIsNotAnError() {
        assertTrue(NativeErrors.isCancellation(CancellationException("x")))
        assertFalse(NativeErrors.isCancellation(RuntimeException("x")))
    }
}
