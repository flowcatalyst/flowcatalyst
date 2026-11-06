package io.flowcatalyst.field

/** Blocking key/value storage for session material. Implementations must be safe to call from any thread. */
interface SecureStorage {
    fun get(key: String): String?
    fun put(key: String, value: String)
    fun remove(key: String)
}

/** For tests and for hosts that supply their own storage. Not persistent. */
class InMemoryStorage : SecureStorage {
    private val map = java.util.concurrent.ConcurrentHashMap<String, String>()
    override fun get(key: String): String? = map[key]
    override fun put(key: String, value: String) { map[key] = value }
    override fun remove(key: String) { map.remove(key) }
    val keys: Set<String> get() = map.keys
}

class DeviceKey(
    val x: String,
    val y: String,
    /** Base64 DER certificates, leaf first; null when there is no attestation. */
    val attestationChain: List<String>?,
)

/** The device's ES256 signing key. Failures are [FcFieldException]. */
interface DeviceKeyProvider {
    fun getOrCreate(): DeviceKey
    /** ES256 over SHA-256(data); returns raw r||s (64 bytes). */
    fun sign(data: ByteArray): ByteArray
    fun delete()
}

/** Shows the app-unlock prompt. Throws [FieldAuthException] (BIOMETRIC_CANCELLED, BIOMETRIC_UNAVAILABLE, ...). */
fun interface UnlockPrompter {
    suspend fun prompt(text: String)
}
