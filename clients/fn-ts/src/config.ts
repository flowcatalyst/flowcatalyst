// ConfigValue / SecretValue (plan §5.3 ops 2-3: config.get, secret.get).
// Both are synchronous: unlike fetch/emit/db (real I/O against Postgres or
// an outbound host), config and secret values are simple in-memory reads
// from the runner's desired state, so there is nothing to await.
import { registry } from "./registry";
import { callHost, utf8Decode } from "./native";
import { OpConfigGet, OpSecretGet } from "./ops";

export class ConfigValue {
  constructor(private readonly _key: string) {}
  get key(): string {
    return this._key;
  }
  /** Fetches the current value from the platform's desired state.
   * Returns undefined when the key has no value bound (distinct from an
   * empty string value). Throws FnError on a host error. */
  get(): string | undefined {
    const { meta, body } = callHost(OpConfigGet, { key: this._key });
    if (meta.found !== true) return undefined;
    return utf8Decode(body);
  }
}

/** Declares that this function reads configuration key `key`. Call it at
 * module top level; the returned handle's get() reads the current value at
 * invocation time. */
export function config(key: string): ConfigValue {
  registry.config.push(key);
  return new ConfigValue(key);
}

export class SecretValue {
  constructor(private readonly _key: string) {}
  get key(): string {
    return this._key;
  }
  /** Fetches the current value. Secret values are never logged by the SDK. */
  get(): string | undefined {
    const { meta, body } = callHost(OpSecretGet, { key: this._key });
    if (meta.found !== true) return undefined;
    return utf8Decode(body);
  }
}

/** Declares that this function reads secret key `key`. */
export function secret(key: string): SecretValue {
  registry.secret.push(key);
  return new SecretValue(key);
}
