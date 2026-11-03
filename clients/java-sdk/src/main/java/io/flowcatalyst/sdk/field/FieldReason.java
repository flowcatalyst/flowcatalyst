package io.flowcatalyst.sdk.field;

/**
 * The stable reasons a field credential is refused. They are the strings the shared fixtures
 * ({@code api/field-fixtures}) expect and the Go, TypeScript and PHP verifiers use.
 */
public final class FieldReason {

    private FieldReason() {}

    public static final String MALFORMED = "malformed";
    public static final String WRONG_TYP = "wrong_typ";
    public static final String WRONG_TOKEN_USE = "wrong_token_use";
    public static final String WRONG_AUD = "wrong_aud";
    public static final String EXPIRED = "expired";
    public static final String INVALID_TOKEN = "invalid_token";
    public static final String PCR = "pcr";
    public static final String MISSING_PROOF = "missing_proof";
    public static final String INVALID_PROOF = "invalid_proof";
    public static final String JKT_MISMATCH = "jkt_mismatch";
    public static final String HTM_MISMATCH = "htm_mismatch";
    public static final String HTU_MISMATCH = "htu_mismatch";
    public static final String ATH_MISMATCH = "ath_mismatch";
    public static final String STALE_PROOF = "stale_proof";
}
