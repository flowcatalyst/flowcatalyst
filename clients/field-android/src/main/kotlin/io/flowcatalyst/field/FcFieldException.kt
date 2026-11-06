package io.flowcatalyst.field

/** `code` is one of KEY_INVALIDATED, BIOMETRIC_UNAVAILABLE, BIOMETRIC_CANCELLED, NO_SECURE_LOCK_SCREEN, INTERNAL. */
class FcFieldException(val code: String, message: String, cause: Throwable? = null) : Exception(message, cause)
