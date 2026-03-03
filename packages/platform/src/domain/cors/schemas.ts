/**
 * CORS Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const CorsOriginAddedDataSchema = Type.Object({
	originId: Type.String(),
	origin: Type.String(),
});

export const CorsOriginDeletedDataSchema = Type.Object({
	originId: Type.String(),
	origin: Type.String(),
});
