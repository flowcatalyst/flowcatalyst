# Changelog

Notable changes to `FlowCatalystField` (field-ios). Releases are tagged
`field-ios/vX.Y.Z` in the platform repository and published to
github.com/flowcatalyst/field-ios.

## 0.5.0

Additive. Needs a platform that has unlock keys (migration 129,
`docs/field-unlock-keys-spec.md`).

### Added (unlock keys)
- Sign in again with Face ID or Touch ID, verified by the platform:
  `unlockKeyAvailability()`, `unlockKey()`, `enableUnlockKey(prompt:)`,
  `loginWithUnlockKey(prompt:)`, `disableUnlockKey()` (and `using:` forms with a
  prompter of your own). The same API, rules and refusal handling as field-android.
- `Session.unlockKeys` (`SessionUnlockKeys`: `allowed`, `attestationRequired`,
  `registerExpiresAt`), the token answer's `unlockKeys`, updated by every sign-in and
  refresh. `unlockKeyAvailability()` is computed from it with no network: the library
  never probes the platform. The register window is counted on the new
  `FieldClock.monotonicMilliseconds()` (`SystemClock`: `CLOCK_MONOTONIC`, which counts
  through sleep; a custom clock defaults to `nowMilliseconds()`).
- `SecureEnclaveUnlockKeyProvider` (a Secure Enclave P-256 key per worker,
  `.privateKeyUsage` + `.biometryCurrentSet`) and `LocalAuthenticationUnlockKeyPrompter`
  (biometrics only, no passcode fallback), wired by `FieldSession.get(config:)`.
- iOS sends no attestation: a client whose policy requires attestation cannot use
  unlock keys on iOS. When the token answer says `attestationRequired`, availability is
  `.attestationRequired` and `enableUnlockKey` refuses `ATTESTATION_REQUIRED` (said so in
  the message) before any key or prompt.
- Types `UnlockKeyAvailability`, `UnlockKeyInfo`, `UnlockKeyPrompt`, `UnlockKeyMaterial`,
  `UnlockKeyCodes`, `UnlockKeyErrorMapping`; protocols `UnlockKeyProvider`,
  `PreparedSignature`, `UnlockKeyPrompter`.
- `FieldEvent.unlockKeyChanged(UnlockKeyInfo?)` (a new case: an exhaustive `switch`
  over `FieldEvent` needs it).
- `Session.amr`: how the login family was signed in (`["hwk","uv"]` after an unlock
  key), stored with the session.
- `FieldSession.init` takes `unlockKeys:` and `unlockKeyPrompter:` (both default nil:
  no unlock keys).
