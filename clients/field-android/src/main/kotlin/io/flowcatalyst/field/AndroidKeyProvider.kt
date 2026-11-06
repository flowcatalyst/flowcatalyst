package io.flowcatalyst.field

/** The Keystore EC P-256 device key (StrongBox first, then TEE) behind [DeviceKeyProvider]. */
internal class AndroidKeyProvider : DeviceKeyProvider {
    private val mgr = DeviceKeyManager()

    @Suppress("UNCHECKED_CAST")
    override fun getOrCreate(): DeviceKey {
        val m = mgr.getOrCreate()
        val jwk = m["jwk"] as Map<String, String>
        val att = m["attestation"] as Map<String, Any?>
        return DeviceKey(jwk.getValue("x"), jwk.getValue("y"), att["chain"] as? List<String>)
    }

    override fun sign(data: ByteArray): ByteArray = mgr.sign(data)
    override fun delete() = mgr.delete()
}
