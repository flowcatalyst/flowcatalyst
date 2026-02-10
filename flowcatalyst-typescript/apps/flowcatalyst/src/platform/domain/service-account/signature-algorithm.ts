/**
 * Signature Algorithm
 *
 * Algorithm used for HMAC webhook signature generation.
 */

export const SignatureAlgorithm = {
  HMAC_SHA256: 'HMAC_SHA256',
} as const;

export type SignatureAlgorithm = (typeof SignatureAlgorithm)[keyof typeof SignatureAlgorithm];
