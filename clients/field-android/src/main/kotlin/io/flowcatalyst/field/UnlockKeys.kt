package io.flowcatalyst.field

import java.security.MessageDigest

// Unlock keys (docs/field-unlock-keys-spec.md section 12): a per-worker ECDSA P-256 key in the phone's secure hardware,
// usable only after a strong biometric, registered with the platform after a PIN sign-in on an enrolled PERSONAL
// device. Signing a fresh platform challenge with it signs the worker in again without the PIN.

/**
 * Whether [FieldSession.enableUnlockKey] can be offered now ([FieldSession.unlockKeyAvailability]). Only [AVAILABLE]
 * means "offer it"; [ENABLED] means this worker already holds a key on this phone.
 */
enum class UnlockKeyAvailability {
    /** Offer "sign in with fingerprint next time". */
    AVAILABLE,

    /** The signed-in worker already holds an unlock key on this phone. */
    ENABLED,

    /** The device is not enrolled. */
    NOT_ENROLLED,

    /** The device is enrolled SHARED: a shared phone's biometrics cannot say whose finger it was. Never offered. */
    SHARED_DEVICE,

    /** Nobody is signed in. */
    NOT_SIGNED_IN,

    /** Someone is acting as the worker (act-as or a console sign-in code). */
    IMPERSONATING,

    /**
     * The session was not signed in with the worker's PIN on this device in the last 10 minutes (a badge alone, an
     * unlock key, a session from before enrolment, a pending PIN change, or too long ago). Offer it after the next PIN
     * sign-in.
     */
    PIN_SIGN_IN_REQUIRED,

    /** Another worker's unlock key is on this phone (one per phone). [FieldSession.disableUnlockKey] hands it over. */
    OTHER_WORKER,

    /** Android older than 11 (API 30), which cannot make a key that needs a strong biometric for every use. */
    OS_TOO_OLD,

    /** The phone's keystore is not backed by secure hardware. */
    NO_SECURE_HARDWARE,

    /** No strong (Class 3) biometric is enrolled on the phone. */
    BIOMETRIC_NOT_ENROLLED,

    /** The phone has no strong biometric hardware, or it cannot be used now. */
    BIOMETRIC_UNAVAILABLE,

    /** The platform said the worker's client has unlock keys switched off (remembered for a day). */
    POLICY_OFF,

    /**
     * The platform refused this phone's key under the client's attestation policy (`422 ATTESTATION_REQUIRED`;
     * remembered for a week). On iOS this is always the answer under `REQUIRED`.
     */
    ATTESTATION_REQUIRED,

    /** No unlock keys in this environment (a browser, or a build without the native library). */
    UNSUPPORTED,
}

/** The unlock key held on this phone (not secret). [pinRequiredBy] is as of the platform's last answer, null for none. */
data class UnlockKeyInfo(
    val unlockKeyId: String,
    val workerId: String,
    val workerCode: String,
    val workerName: String,
    val deviceId: String,
    /** ms since the epoch. */
    val createdAt: Long,
    /** When the platform will next ask for the PIN (ms since the epoch); null when the policy forces no periodic PIN. */
    val pinRequiredBy: Long?,
    /** The platform's attestation verdict at registration: NONE, MALFORMED, UNTRUSTED or TRUSTED. */
    val attestationVerdict: String,
)

/** The words of the biometric prompt. [cancel] is the prompt's button ("Use PIN"); the library has no words of its own. */
data class UnlockKeyPrompt(
    val title: String,
    val cancel: String,
    val subtitle: String? = null,
)

/** A new unlock key's public half, its attestation chain (base64 DER, leaf first) and where it lives. */
class UnlockKeyMaterial(
    val x: String,
    val y: String,
    val attestationChain: List<String>?,
    /** STRONGBOX, TEE, SOFTWARE or NONE (unknown). */
    val level: String,
)

/** One signature over an unlock key, readied without a prompt; [sign] works once, after the prompt authorised it. */
interface PreparedSignature {
    /** ES256 over SHA-256(data); returns raw r||s (64 bytes). Failures are [FcFieldException]. */
    fun sign(data: ByteArray): ByteArray
}

/** The phone's unlock keys (the Android Keystore; fakes in tests). Failures are [FcFieldException]. */
interface UnlockKeyProvider {
    /** [UnlockKeyAvailability.AVAILABLE], `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_UNAVAILABLE` or `OS_TOO_OLD`. */
    fun biometricStatus(): UnlockKeyAvailability

    /** Makes a new key under [alias] bound to the current strong biometrics, attested with [attestationChallenge]. */
    fun create(alias: String, attestationChallenge: ByteArray): UnlockKeyMaterial

    /** Readies [alias] for one signature, no prompt. `KEY_INVALIDATED` when the phone invalidated the key or it is gone. */
    fun prepare(alias: String): PreparedSignature

    fun delete(alias: String)
}

/** Shows the biometric prompt over [signature] and returns its signature of [data] (raw r||s). Throws [FieldAuthException]. */
fun interface UnlockKeyPrompter {
    suspend fun authorise(prompt: UnlockKeyPrompt, signature: PreparedSignature, data: ByteArray): ByteArray
}

/** For hosts without unlock keys (and the default in tests): every call refuses. */
internal object NoUnlockKeys : UnlockKeyProvider {
    override fun biometricStatus() = UnlockKeyAvailability.UNSUPPORTED
    override fun create(alias: String, attestationChallenge: ByteArray): UnlockKeyMaterial =
        throw FcFieldException("UNLOCK_KEYS_UNSUPPORTED", "no unlock keys here")
    override fun prepare(alias: String): PreparedSignature = throw FcFieldException("KEY_INVALIDATED", "no unlock keys here")
    override fun delete(alias: String) {}
}

/** The unlock-key JWS of section 3 (pure, tested against api/field-fixtures/unlock.json). */
internal object UnlockJws {
    const val KEY_PROOF_TYPE = "fc-unlock-key+jwt"
    const val ASSERTION_TYPE = "fc-unlock+jwt"
    const val REGISTER_PATH = "/field/auth/unlock-keys"
    const val CHALLENGE_PATH = "/field/auth/unlock/challenge"
    const val UNLOCK_PATH = "/field/auth/unlock"
    const val REVOKE_PATH = "/field/auth/unlock-keys/revoke"

    /** `base64url(header) . base64url(claims)` of a key proof, members in the spec's order. */
    fun keyProofSigningInput(baseUrl: String, nonce: String, workerId: String, deviceId: String, iat: Long): String =
        signingInput(
            listOf("typ" to KEY_PROOF_TYPE, "alg" to "ES256"),
            listOf(
                "aud" to baseUrl.trimEnd('/') + REGISTER_PATH, "nonce" to nonce, "sub" to workerId,
                "device_id" to deviceId, "iat" to iat,
            ),
        )

    /** `base64url(header) . base64url(claims)` of an unlock assertion, members in the spec's order. */
    fun assertionSigningInput(
        baseUrl: String, unlockKeyId: String, challenge: String, workerId: String, deviceId: String, appCode: String, iat: Long,
    ): String = signingInput(
        listOf("typ" to ASSERTION_TYPE, "alg" to "ES256", "kid" to unlockKeyId),
        listOf(
            "aud" to baseUrl.trimEnd('/') + UNLOCK_PATH, "nonce" to challenge, "sub" to workerId,
            "device_id" to deviceId, "app" to appCode, "iat" to iat,
        ),
    )

    /** The compact JWS: the signing input and the raw 64-byte signature. */
    fun compact(signingInput: String, rawSignature: ByteArray): String {
        if (rawSignature.size != 64) throw FieldAuthException("INTERNAL", "the unlock key returned a malformed signature")
        return signingInput + "." + Dpop.b64u(rawSignature)
    }

    /** SHA-256 of `fc-unlock-key-attestation:v1\n<sid>\n<workerId>\n<deviceId>` (UTF-8), for setAttestationChallenge. */
    fun attestationChallenge(sid: String, workerId: String, deviceId: String): ByteArray =
        MessageDigest.getInstance("SHA-256")
            .digest("fc-unlock-key-attestation:v1\n$sid\n$workerId\n$deviceId".toByteArray(Charsets.UTF_8))

    private fun signingInput(header: List<Pair<String, Any>>, claims: List<Pair<String, Any>>): String =
        Dpop.b64u(json(header).toByteArray(Charsets.UTF_8)) + "." + Dpop.b64u(json(claims).toByteArray(Charsets.UTF_8))

    /**
     * A JSON object with its members in the given order and no escapes beyond what JSON needs (org.json on Android
     * writes `/` as `\/` and does not keep member order on the JVM, so it cannot produce the vectors' bytes).
     */
    fun json(members: List<Pair<String, Any>>): String = buildString {
        append('{')
        members.forEachIndexed { i, (k, v) ->
            if (i > 0) append(',')
            string(k)
            append(':')
            when (v) {
                is String -> string(v)
                is Long, is Int -> append(v.toString())
                else -> throw IllegalArgumentException("unsupported JSON value")
            }
        }
        append('}')
    }

    private fun StringBuilder.string(s: String) {
        append('"')
        for (c in s) {
            when {
                c == '"' -> append("\\\"")
                c == '\\' -> append("\\\\")
                c == '\n' -> append("\\n")
                c == '\r' -> append("\\r")
                c == '\t' -> append("\\t")
                c < ' ' -> append(String.format("\\u%04x", c.code))
                else -> append(c)
            }
        }
        append('"')
    }
}

/** The library's own codes for unlock keys (the platform's codes pass through as they are). */
object UnlockKeyCodes {
    /** No unlock key is held on this phone: sign in with the PIN. */
    const val NO_UNLOCK_KEY = "NO_UNLOCK_KEY"

    /** The phone invalidated the key (a biometric was enrolled or removed, or the keystore lost it): PIN, and offer again. */
    const val UNLOCK_KEY_INVALIDATED = "UNLOCK_KEY_INVALIDATED"

    /** This phone or environment cannot hold an unlock key (old Android, no secure hardware, a browser). */
    const val UNLOCK_KEYS_UNSUPPORTED = "UNLOCK_KEYS_UNSUPPORTED"

    /** No strong biometric is enrolled on the phone. */
    const val BIOMETRIC_NOT_ENROLLED = "BIOMETRIC_NOT_ENROLLED"

    /** Too many failed biometric tries: the phone locked its biometric for a while (or until its own PIN is entered). */
    const val BIOMETRIC_LOCKOUT = "BIOMETRIC_LOCKOUT"

    /** Another worker's unlock key is on this phone; disable it first (a hand-over), or let that worker keep it. */
    const val UNLOCK_KEY_OTHER_WORKER = "UNLOCK_KEY_OTHER_WORKER"
}
