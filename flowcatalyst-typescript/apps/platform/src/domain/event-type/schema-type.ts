/**
 * Schema Type
 *
 * The format of a spec version's schema content.
 */

export type SchemaType = 'JSON_SCHEMA' | 'PROTO' | 'XSD';

export const SchemaType = {
	JSON_SCHEMA: 'JSON_SCHEMA' as const,
	PROTO: 'PROTO' as const,
	XSD: 'XSD' as const,
} as const;
