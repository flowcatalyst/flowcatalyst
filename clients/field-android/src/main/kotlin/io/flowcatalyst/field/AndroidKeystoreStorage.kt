package io.flowcatalyst.field

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Session storage: AES-256-GCM under a Keystore key WITHOUT user authentication (StrongBox first, then TEE), so the
 * session can refresh in the background and after a reboot. Ciphertext lives in SharedPreferences as
 * "v1:<iv b64>:<ciphertext b64>". The entry name is bound in as AAD. A value that can no longer be decrypted
 * (key lost, restore from backup) reads as absent. Never logs keys or values.
 */
internal class AndroidKeystoreStorage(context: Context) : SecureStorage {
    private val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    @Synchronized
    override fun get(key: String): String? {
        val s = prefs.getString(key, null) ?: return null
        val p = s.split(":")
        if (p.size != 3 || p[0] != "v1") return null
        return try {
            val d = Base64.getDecoder()
            val cipher = Cipher.getInstance(TRANSFORM)
            cipher.init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(128, d.decode(p[1])))
            cipher.updateAAD(key.toByteArray(Charsets.UTF_8))
            String(cipher.doFinal(d.decode(p[2])), Charsets.UTF_8)
        } catch (e: Exception) {
            prefs.edit().remove(key).commit()
            null
        }
    }

    @Synchronized
    override fun put(key: String, value: String) {
        try {
            val cipher = Cipher.getInstance(TRANSFORM)
            try {
                cipher.init(Cipher.ENCRYPT_MODE, secretKey())
            } catch (e: Exception) {
                runCatching { keyStore().deleteEntry(ALIAS) } // unusable key: start over
                cipher.init(Cipher.ENCRYPT_MODE, secretKey())
            }
            cipher.updateAAD(key.toByteArray(Charsets.UTF_8))
            val ct = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
            val e = Base64.getEncoder()
            prefs.edit().putString(key, "v1:${e.encodeToString(cipher.iv)}:${e.encodeToString(ct)}").commit()
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not store value", e)
        }
    }

    @Synchronized
    override fun remove(key: String) {
        prefs.edit().remove(key).commit()
    }

    private fun keyStore(): KeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    private fun secretKey(): SecretKey {
        (keyStore().getKey(ALIAS, null) as? SecretKey)?.let { return it }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            try {
                return generate(strongBox = true)
            } catch (e: StrongBoxUnavailableException) {
                // fall through to the TEE
            } catch (e: java.security.ProviderException) {
                // some OEMs throw this instead
            }
        }
        return generate(strongBox = false)
    }

    private fun generate(strongBox: Boolean): SecretKey {
        val b = KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .setUserAuthenticationRequired(false)
        if (strongBox && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) b.setIsStrongBoxBacked(true)
        val kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        kg.init(b.build())
        return kg.generateKey()
    }

    companion object {
        const val PREFS = "fc_field_session_store"
        const val ALIAS = "fc_field_session_key"
        private const val TRANSFORM = "AES/GCM/NoPadding"
    }
}
