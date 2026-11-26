import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import { FieldAuthError } from "../../src/index.js";
import { enrolledAndLoggedIn, rig } from "./fakes.js";

// Unlock keys are not offered in a browser (docs/field-unlock-keys-spec.md sections 6.1 and 12.5).
describe("unlock keys in a browser", () => {
	it("are never available, even signed in on an enrolled personal device", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		assert.equal(await r.auth.unlockKeyAvailability(), "UNSUPPORTED");
		assert.equal(await r.auth.unlockKeyAvailability({ askPlatform: true }), "UNSUPPORTED");
		assert.equal(await r.auth.unlockKey(), null);
	});

	it("refuse with UNLOCK_KEYS_UNSUPPORTED and send nothing", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const before = r.platform.log.length;
		for (const call of [() => r.auth.enableUnlockKey(), () => r.auth.loginWithUnlockKey()]) {
			await assert.rejects(call(), (e: unknown) => {
				assert.ok(e instanceof FieldAuthError);
				assert.equal(e.code, "UNLOCK_KEYS_UNSUPPORTED");
				return true;
			});
		}
		await r.auth.disableUnlockKey();
		assert.equal(r.platform.log.length, before);
	});
});
