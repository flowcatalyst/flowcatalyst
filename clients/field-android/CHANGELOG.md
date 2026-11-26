# Changelog

Notable changes to `io.flowcatalyst:field-android`. Releases are tagged `field-android/vX.Y.Z` in the platform
repository (`make release-field-android BUMP=…`).

## 0.6.0

Additive. Needs a platform whose token answers carry `unlockKeys` (`docs/field-unlock-keys-spec.md` section 2.3);
against an older one availability is `POLICY_OFF` and nothing is offered.

### Added (unlock keys, `docs/field-unlock-keys-spec.md` section 12)
- `FieldSession.unlockKeyAvailability()`, `unlockKey()`, `enableUnlockKey(activity, prompt)`,
  `loginWithUnlockKey(activity, prompt)` and `disableUnlockKey()`: a per-worker Keystore key behind a strong
  biometric, registered after a PIN sign-in on an enrolled PERSONAL phone, that signs the worker in again without the
  PIN (README.md, "Unlock keys"). Android 11 (API 30) and later.
- `Session.unlockKeys` (`SessionUnlockKeys(allowed, attestationRequired, registerExpiresAt)`): the token answer's
  `unlockKeys`, read at every sign-in and refresh; null from a platform that sends none. Availability is computed
  from it on the phone, with no request; the register window is counted on the monotonic clock.
- Types `UnlockKeyAvailability`, `UnlockKeyInfo`, `UnlockKeyPrompt`, `UnlockKeyCodes`, `SessionUnlockKeys`; the SPI
  `UnlockKeyProvider`, `PreparedSignature`, `UnlockKeyPrompter`.
- `FieldEvent.UnlockKeyChanged(key)`. A `when` over `FieldEvent` that names every member needs the new one.
- `Session.amr` (the access token's `amr` claim), stored with the session; empty for a session stored before.
- New codes: `NO_UNLOCK_KEY`, `UNLOCK_KEY_INVALIDATED`, `UNLOCK_KEYS_UNSUPPORTED`, `BIOMETRIC_NOT_ENROLLED`,
  `BIOMETRIC_LOCKOUT`, `UNLOCK_KEY_OTHER_WORKER`; the platform's unlock-key codes pass through.

### Unchanged
- `unlock(activity, prompt)`, the app unlock, works as before and is separate from unlock keys.
