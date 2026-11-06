package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Adds `Authorization: DPoP` and a proof to every request. Retries once on `401 use_dpop_nonce` (with the fresh
 * nonce) and once on `401 invalid_token` (after a refresh). A refresh that fails with a session-ending code surfaces
 * as a [FieldAuthException] (an IOException) after the session emitted SessionEnded. Runs on OkHttp's threads; blocks
 * the calling thread while a refresh is in flight, so never call it on the main thread.
 */
internal class FieldInterceptor(private val session: FieldSession) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val original = chain.request()
        val oneShot = original.body?.isOneShot() == true
        var nonceRetried = false
        var refreshRetried = false
        while (true) {
            val signed = runBlocking { session.signedFor(original.method, original.url.toString()) }
            val req = original.newBuilder()
                .header("Authorization", "DPoP ${signed.accessToken}")
                .header("DPoP", signed.proof)
                .build()
            val resp = chain.proceed(req)
            resp.header("DPoP-Nonce")?.let { session.noteNonce(original.url.toString(), it) }
            if (resp.code != 401) return resp
            val www = resp.header("WWW-Authenticate") ?: ""
            if (www.contains("use_dpop_nonce") && !nonceRetried && !oneShot) {
                nonceRetried = true
                resp.close()
                continue
            }
            if (www.contains("invalid_token")) {
                if (!refreshRetried && !oneShot) {
                    refreshRetried = true
                    resp.close()
                    runBlocking { session.refreshIfStale(signed.accessToken) }
                    continue
                }
                session.emitSessionEnded("invalid_token")
            }
            return resp
        }
    }
}
