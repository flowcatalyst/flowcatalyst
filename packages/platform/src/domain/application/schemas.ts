/**
 * Application Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { ApplicationTypeSchema } from "../shared-schemas.js";

export const ApplicationCreatedDataSchema = Type.Object({
	applicationId: Type.String(),
	type: ApplicationTypeSchema,
	code: Type.String(),
	name: Type.String(),
});

export const ApplicationUpdatedDataSchema = Type.Object({
	applicationId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	previousName: Type.String(),
});

export const ApplicationActivatedDataSchema = Type.Object({
	applicationId: Type.String(),
	code: Type.String(),
});

export const ApplicationDeactivatedDataSchema = Type.Object({
	applicationId: Type.String(),
	code: Type.String(),
});

export const ApplicationDeletedDataSchema = Type.Object({
	applicationId: Type.String(),
	code: Type.String(),
	name: Type.String(),
});

export const ApplicationEnabledForClientDataSchema = Type.Object({
	applicationId: Type.String(),
	clientId: Type.String(),
	configId: Type.String(),
});

export const ApplicationDisabledForClientDataSchema = Type.Object({
	applicationId: Type.String(),
	clientId: Type.String(),
	configId: Type.String(),
});
