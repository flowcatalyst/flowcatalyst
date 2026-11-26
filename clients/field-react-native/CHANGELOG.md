# Changelog

Notable changes to `@flowcatalyst/field-react-native`. Releases are tagged `field-react-native/vX.Y.Z` in the
platform repository (`make release-field-rn BUMP=…`) and published to github.com/flowcatalyst/field-react-native.

## Unreleased

Additive. Needs field-android and field-ios with unlock keys (built here against the sibling directories) and a
platform with unlock keys (migration 129).

### Added (unlock keys, `docs/field-unlock-keys-spec.md` section 12)
- `FieldClient.unlockKeyAvailability({askPlatform})`, `unlockKey()`, `enableUnlockKey(prompt)`,
  `loginWithUnlockKey(prompt)`, `disableUnlockKey()` (README.md, "Unlock keys"), over the same TurboModule methods on
  Android (field-android `FieldSession`) and iOS (field-ios `FieldSession`).
- Types `UnlockKeyAvailability`, `UnlockKeyInfo`, `UnlockKeyPrompt`; constants `UNLOCK_KEY_AVAILABILITIES`,
  `UNLOCK_KEY_CODES`; event `UnlockKeyChanged {key}`; `Session.amr`.
- Against a native module built before unlock keys the client answers `UNSUPPORTED` / `UNLOCK_KEYS_UNSUPPORTED`.

### Unchanged
- The Expo config plugin: unlock keys need nothing it does not already add (`USE_BIOMETRIC`,
  `NSFaceIDUsageDescription`). Its default Face ID text speaks of unlocking the app; pass `faceIdPermission` to cover
  signing in.
- `unlock(prompt)`, the app unlock.
