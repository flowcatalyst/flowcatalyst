globalThis.__fc_utf8_encode = (s) => new TextEncoder().encode(s);
globalThis.__fc_utf8_decode = (b) => new TextDecoder().decode(b);
globalThis.__fc_call = () => ({ code: 0, meta: "{}", body: new Uint8Array(0) });
