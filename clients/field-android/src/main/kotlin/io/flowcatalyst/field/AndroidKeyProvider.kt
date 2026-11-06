package io.flowcatalyst.field

/** The Keystore EC P-256 device key (StrongBox first, then TEE) behind [DeviceKeyProvider]. */
class AndroidKeyProvider : DeviceKeyProvider {
    // One manager per process: every provider instance (the session's, a React Native bridge's) shares its lock.
    private val mgr = shared

    @Suppress("UNCHECKED_CAST")
    override fun getOrCreate(): DeviceKey {
        val m = mgr.getOrCreate()
        val jwk = m["jwk"] as Map<String, String>
        val att = m["attestation"] as Map<String, Any?>
        return DeviceKey(jwk.getValue("x"), jwk.getValue("y"), att["chain"] as? List<String>, att["level"] as? String ?: "NONE")
    }

    override fun sign(data: ByteArray): ByteArray = mgr.sign(data)
    override fun delete() = mgr.delete()

    private companion object {
        val shared = DeviceKeyManager()
    }
}
