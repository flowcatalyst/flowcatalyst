# Changelog

Notable changes to `io.flowcatalyst:field-android`. Releases are tagged `field-android/vX.Y.Z` in the platform
repository (`make release-field-android BUMP=…`).

## Unreleased

Additive. Needs a platform with unlock keys (migration 129); an older one answers the new calls with `404`.

### Added (unlock keys, `docs/field-unlock-keys-spec.md` section 12)
- `FieldSession.unlockKeyAvailability(askPlatform)`, `unlockKey()`, `enableUnlockKey(activity, prompt)`,
  `loginWithUnlockKey(activity, prompt)` and `disableUnlockKey()`: a per-worker Keystore key behind a strong
  biometric, registered after a PIN sign-in on an enrolled PERSONAL phone, that signs the worker in again without the
  PIN (README.md, "Unlock keys"). Android 11 (API 30) and later.
- Types `UnlockKeyAvailability`, `UnlockKeyInfo`, `UnlockKeyPrompt`, `UnlockKeyCodes`; the SPI `UnlockKeyProvider`,
  `PreparedSignature`, `UnlockKeyPrompter`.
- `FieldEvent.UnlockKeyChanged(key)`. A `when` over `FieldEvent` that names every member needs the new one.
- `Session.amr` (the access token's `amr` claim), stored with the session; empty for a session stored before.
- New codes: `NO_UNLOCK_KEY`, `UNLOCK_KEY_INVALIDATED`, `UNLOCK_KEYS_UNSUPPORTED`, `BIOMETRIC_NOT_ENROLLED`,
  `BIOMETRIC_LOCKOUT`, `UNLOCK_KEY_OTHER_WORKER`; the platform's unlock-key codes pass through.

### Unchanged
- `unlock(activity, prompt)`, the app unlock, works as before and is separate from unlock keys.
