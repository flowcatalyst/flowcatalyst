# Changelog

Notable changes to `@flowcatalyst/field-js`. Releases are tagged `field-js/vX.Y.Z` in the platform repository
(`make release-field-js BUMP=…`) and published to github.com/flowcatalyst/field-js.

## 0.6.0

### Added
- Unlock keys are not offered in a browser (`docs/field-unlock-keys-spec.md` sections 6.1 and 12.5), and the library
  says so with the native libraries' names: `FieldAuth.unlockKeyAvailability()` is always `"UNSUPPORTED"`,
  `unlockKey()` is `null`, `enableUnlockKey()` and `loginWithUnlockKey()` reject with `UNLOCK_KEYS_UNSUPPORTED`,
  `disableUnlockKey()` does nothing. Type `UnlockKeyAvailability`; error code `UNLOCK_KEYS_UNSUPPORTED`.
- `Session.unlockKeys` (`SessionUnlockKeys`: `allowed`, `attestationRequired`, `registerExpiresAt`): the token
  answer's `unlockKeys`, kept for parity with the native libraries (null when the platform sends none). A browser
  offers no unlock keys whatever it says.
