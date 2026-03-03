/**
 * Anchor Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const AnchorDomainCreatedDataSchema = Type.Object({
	anchorDomainId: Type.String(),
	domain: Type.String(),
});

export const AnchorDomainUpdatedDataSchema = Type.Object({
	anchorDomainId: Type.String(),
	domain: Type.String(),
	previousDomain: Type.String(),
});

export const AnchorDomainDeletedDataSchema = Type.Object({
	anchorDomainId: Type.String(),
	domain: Type.String(),
});
