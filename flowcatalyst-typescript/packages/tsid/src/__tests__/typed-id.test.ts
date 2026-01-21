import { describe, it, expect } from 'vitest';
import { generate } from '../tsid.js';
import {
	EntityType,
	serialize,
	serializeAll,
	deserialize,
	deserializeAll,
	deserializeOrNull,
	isValidTypedId,
	parseAny,
	getPrefix,
	getTypeFromPrefix,
	stripPrefix,
	ensurePrefix,
	TypedIdError,
	type TypedIdErrorReason,
} from '../typed-id.js';

describe('serialize', () => {
	it('should add the correct prefix for CLIENT', () => {
		const id = generate();
		const external = serialize('CLIENT', id);
		expect(external).toBe(`client_${id}`);
	});

	it('should add the correct prefix for each entity type', () => {
		const id = generate();

		expect(serialize('CLIENT', id)).toBe(`client_${id}`);
		expect(serialize('PRINCIPAL', id)).toBe(`principal_${id}`);
		expect(serialize('APPLICATION', id)).toBe(`app_${id}`);
		expect(serialize('ROLE', id)).toBe(`role_${id}`);
		expect(serialize('PERMISSION', id)).toBe(`perm_${id}`);
		expect(serialize('OAUTH_CLIENT', id)).toBe(`oauth_${id}`);
		expect(serialize('AUTH_CODE', id)).toBe(`authcode_${id}`);
		expect(serialize('CLIENT_AUTH_CONFIG', id)).toBe(`authcfg_${id}`);
		expect(serialize('APP_CLIENT_CONFIG', id)).toBe(`appcfg_${id}`);
		expect(serialize('IDP_ROLE_MAPPING', id)).toBe(`idpmap_${id}`);
		expect(serialize('CORS_ORIGIN', id)).toBe(`cors_${id}`);
		expect(serialize('ANCHOR_DOMAIN', id)).toBe(`anchor_${id}`);
		expect(serialize('CLIENT_ACCESS_GRANT', id)).toBe(`grant_${id}`);
		expect(serialize('AUDIT_LOG', id)).toBe(`audit_${id}`);
		expect(serialize('EVENT_TYPE', id)).toBe(`evtype_${id}`);
		expect(serialize('EVENT', id)).toBe(`event_${id}`);
		expect(serialize('SUBSCRIPTION', id)).toBe(`sub_${id}`);
		expect(serialize('DISPATCH_POOL', id)).toBe(`pool_${id}`);
		expect(serialize('DISPATCH_JOB', id)).toBe(`job_${id}`);
		expect(serialize('SERVICE_ACCOUNT', id)).toBe(`svc_${id}`);
	});
});

describe('deserialize', () => {
	it('should strip the correct prefix', () => {
		const id = generate();
		const external = `client_${id}`;
		const internal = deserialize('CLIENT', external);
		expect(internal).toBe(id);
	});

	it('should throw for wrong prefix type', () => {
		const id = generate();
		const external = `client_${id}`;

		expect(() => deserialize('APPLICATION', external)).toThrow(TypedIdError);
		expect(() => deserialize('APPLICATION', external)).toThrow('Expected APPLICATION ID');
	});

	it('should throw for missing prefix', () => {
		const id = generate();

		expect(() => deserialize('CLIENT', id)).toThrow(TypedIdError);
		expect(() => deserialize('CLIENT', id)).toThrow("expected prefix 'client_'");
	});

	it('should throw for invalid TSID format', () => {
		expect(() => deserialize('CLIENT', 'client_invalid')).toThrow(TypedIdError);
		expect(() => deserialize('CLIENT', 'client_invalid')).toThrow('Invalid TSID format');
	});
});

describe('deserializeOrNull', () => {
	it('should return null for invalid input', () => {
		expect(deserializeOrNull('CLIENT', 'invalid')).toBeNull();
		expect(deserializeOrNull('CLIENT', 'app_0HZXEQ5Y8JY5Z')).toBeNull();
	});

	it('should return null for null/undefined input', () => {
		expect(deserializeOrNull('CLIENT', null)).toBeNull();
		expect(deserializeOrNull('CLIENT', undefined)).toBeNull();
	});

	it('should return the ID for valid input', () => {
		const id = generate();
		const external = `client_${id}`;
		expect(deserializeOrNull('CLIENT', external)).toBe(id);
	});
});

describe('isValidTypedId', () => {
	it('should return true for valid typed ID', () => {
		const id = generate();
		const external = `client_${id}`;
		expect(isValidTypedId('CLIENT', external)).toBe(true);
	});

	it('should return false for wrong type', () => {
		const id = generate();
		const external = `client_${id}`;
		expect(isValidTypedId('APPLICATION', external)).toBe(false);
	});

	it('should return false for invalid format', () => {
		expect(isValidTypedId('CLIENT', 'invalid')).toBe(false);
	});
});

describe('parseAny', () => {
	it('should parse prefixed IDs', () => {
		const id = generate();
		const result = parseAny(`client_${id}`);
		expect(result.type).toBe('CLIENT');
		expect(result.id).toBe(id);
	});

	it('should handle unprefixed IDs', () => {
		const id = generate();
		const result = parseAny(id);
		expect(result.type).toBeNull();
		expect(result.id).toBe(id);
	});

	it('should handle unknown prefixes', () => {
		const id = generate();
		const result = parseAny(`unknown_${id}`);
		expect(result.type).toBeNull();
		expect(result.id).toBe(id);
	});
});

describe('getPrefix', () => {
	it('should return the correct prefix', () => {
		expect(getPrefix('CLIENT')).toBe('client');
		expect(getPrefix('APPLICATION')).toBe('app');
		expect(getPrefix('OAUTH_CLIENT')).toBe('oauth');
	});
});

describe('getTypeFromPrefix', () => {
	it('should return the correct type', () => {
		expect(getTypeFromPrefix('client')).toBe('CLIENT');
		expect(getTypeFromPrefix('app')).toBe('APPLICATION');
		expect(getTypeFromPrefix('oauth')).toBe('OAUTH_CLIENT');
	});

	it('should return null for unknown prefix', () => {
		expect(getTypeFromPrefix('unknown')).toBeNull();
	});
});

describe('stripPrefix', () => {
	it('should strip prefix from prefixed ID', () => {
		const id = generate();
		expect(stripPrefix(`client_${id}`)).toBe(id);
	});

	it('should return unprefixed ID as-is', () => {
		const id = generate();
		expect(stripPrefix(id)).toBe(id);
	});
});

describe('ensurePrefix', () => {
	it('should add prefix to unprefixed ID', () => {
		const id = generate();
		expect(ensurePrefix('CLIENT', id)).toBe(`client_${id}`);
	});

	it('should keep correctly prefixed ID', () => {
		const id = generate();
		const external = `client_${id}`;
		expect(ensurePrefix('CLIENT', external)).toBe(external);
	});

	it('should throw for ID with wrong prefix', () => {
		const id = generate();
		const external = `app_${id}`;
		expect(() => ensurePrefix('CLIENT', external)).toThrow(TypedIdError);
	});
});

describe('EntityType constants', () => {
	it('should have all expected entity types', () => {
		expect(EntityType.CLIENT).toBe('client');
		expect(EntityType.PRINCIPAL).toBe('principal');
		expect(EntityType.APPLICATION).toBe('app');
		expect(EntityType.ROLE).toBe('role');
		expect(EntityType.PERMISSION).toBe('perm');
		expect(EntityType.OAUTH_CLIENT).toBe('oauth');
		expect(EntityType.AUTH_CODE).toBe('authcode');
		expect(EntityType.CLIENT_AUTH_CONFIG).toBe('authcfg');
		expect(EntityType.APP_CLIENT_CONFIG).toBe('appcfg');
		expect(EntityType.IDP_ROLE_MAPPING).toBe('idpmap');
		expect(EntityType.CORS_ORIGIN).toBe('cors');
		expect(EntityType.ANCHOR_DOMAIN).toBe('anchor');
		expect(EntityType.CLIENT_ACCESS_GRANT).toBe('grant');
		expect(EntityType.AUDIT_LOG).toBe('audit');
		expect(EntityType.EVENT_TYPE).toBe('evtype');
		expect(EntityType.EVENT).toBe('event');
		expect(EntityType.SUBSCRIPTION).toBe('sub');
		expect(EntityType.DISPATCH_POOL).toBe('pool');
		expect(EntityType.DISPATCH_JOB).toBe('job');
		expect(EntityType.SERVICE_ACCOUNT).toBe('svc');
	});
});

describe('serialize null handling', () => {
	it('should return null for null input', () => {
		expect(serialize('CLIENT', null)).toBeNull();
	});

	it('should return null for undefined input', () => {
		expect(serialize('CLIENT', undefined)).toBeNull();
	});

	it('should return string for string input', () => {
		const id = generate();
		const result = serialize('CLIENT', id);
		expect(result).toBe(`client_${id}`);
	});
});

describe('serializeAll', () => {
	it('should serialize multiple IDs', () => {
		const ids = [generate(), generate(), generate()];
		const externals = serializeAll('CLIENT', ids);
		expect(externals).toHaveLength(3);
		expect(externals[0]).toBe(`client_${ids[0]}`);
		expect(externals[1]).toBe(`client_${ids[1]}`);
		expect(externals[2]).toBe(`client_${ids[2]}`);
	});

	it('should handle empty array', () => {
		const externals = serializeAll('CLIENT', []);
		expect(externals).toHaveLength(0);
	});
});

describe('deserializeAll', () => {
	it('should deserialize multiple IDs', () => {
		const ids = [generate(), generate(), generate()];
		const externals = ids.map((id) => `client_${id}`);
		const internals = deserializeAll('CLIENT', externals);
		expect(internals).toHaveLength(3);
		expect(internals[0]).toBe(ids[0]);
		expect(internals[1]).toBe(ids[1]);
		expect(internals[2]).toBe(ids[2]);
	});

	it('should handle empty array', () => {
		const internals = deserializeAll('CLIENT', []);
		expect(internals).toHaveLength(0);
	});

	it('should throw on first invalid ID', () => {
		const ids = [generate(), generate()];
		const externals = [`client_${ids[0]}`, 'invalid', `client_${ids[1]}`];
		expect(() => deserializeAll('CLIENT', externals)).toThrow(TypedIdError);
	});
});

describe('TypedIdError reason codes', () => {
	it('should have reason "empty" for null/blank input', () => {
		try {
			deserialize('CLIENT', '');
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('empty');
		}
	});

	it('should have reason "empty" for whitespace-only input', () => {
		try {
			deserialize('CLIENT', '   ');
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('empty');
		}
	});

	it('should have reason "missing_separator" for ID without underscore', () => {
		try {
			deserialize('CLIENT', generate());
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('missing_separator');
		}
	});

	it('should have reason "type_mismatch" for wrong entity type', () => {
		const id = generate();
		try {
			deserialize('APPLICATION', `client_${id}`);
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('type_mismatch');
			expect((e as TypedIdError).expectedType).toBe('APPLICATION');
			expect((e as TypedIdError).actualType).toBe('CLIENT');
		}
	});

	it('should have reason "unknown_prefix" for unrecognized prefix', () => {
		const id = generate();
		try {
			deserialize('CLIENT', `unknown_${id}`);
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('unknown_prefix');
		}
	});

	it('should have reason "invalid_tsid" for invalid TSID format', () => {
		try {
			deserialize('CLIENT', 'client_invalid');
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('invalid_tsid');
		}
	});

	it('should have reason "type_mismatch" in ensurePrefix for wrong type', () => {
		const id = generate();
		try {
			ensurePrefix('CLIENT', `app_${id}`);
		} catch (e) {
			expect(e).toBeInstanceOf(TypedIdError);
			expect((e as TypedIdError).reason).toBe('type_mismatch');
		}
	});
});
