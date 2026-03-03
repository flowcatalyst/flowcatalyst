/**
 * Subscription Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { EventTypeBindingSchema } from "../shared-schemas.js";

export const SubscriptionCreatedDataSchema = Type.Object({
	subscriptionId: Type.String(),
	code: Type.String(),
	applicationCode: Type.Union([Type.String(), Type.Null()]),
	name: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	clientScoped: Type.Boolean(),
	eventTypes: Type.Array(EventTypeBindingSchema),
	target: Type.String(),
});

export const SubscriptionUpdatedDataSchema = Type.Object({
	subscriptionId: Type.String(),
	code: Type.String(),
	applicationCode: Type.Union([Type.String(), Type.Null()]),
	name: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	eventTypes: Type.Array(EventTypeBindingSchema),
	target: Type.String(),
});

export const SubscriptionDeletedDataSchema = Type.Object({
	subscriptionId: Type.String(),
	code: Type.String(),
	applicationCode: Type.Union([Type.String(), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
});

export const SubscriptionsSyncedDataSchema = Type.Object({
	applicationCode: Type.String(),
	subscriptionsCreated: Type.Integer(),
	subscriptionsUpdated: Type.Integer(),
	subscriptionsDeleted: Type.Integer(),
	syncedSubscriptionCodes: Type.Array(Type.String()),
});
