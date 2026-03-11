import { defineRelations } from "drizzle-orm";
import * as schema from "./schema";

export const relations = defineRelations(schema, (r) => ({
	oauthClientAllowedOrigins: {
		oauthClient: r.one.oauthClients({
			from: r.oauthClientAllowedOrigins.oauthClientId,
			to: r.oauthClients.id
		}),
	},
	oauthClients: {
		oauthClientAllowedOrigins: r.many.oauthClientAllowedOrigins(),
		oauthClientApplicationIds: r.many.oauthClientApplicationIds(),
		oauthClientGrantTypes: r.many.oauthClientGrantTypes(),
		oauthClientRedirectUrises: r.many.oauthClientRedirectUris(),
	},
	oauthClientApplicationIds: {
		oauthClient: r.one.oauthClients({
			from: r.oauthClientApplicationIds.oauthClientId,
			to: r.oauthClients.id
		}),
	},
	oauthClientGrantTypes: {
		oauthClient: r.one.oauthClients({
			from: r.oauthClientGrantTypes.oauthClientId,
			to: r.oauthClients.id
		}),
	},
	oauthClientRedirectUris: {
		oauthClient: r.one.oauthClients({
			from: r.oauthClientRedirectUris.oauthClientId,
			to: r.oauthClients.id
		}),
	},
	principalRoles: {
		principal: r.one.principals({
			from: r.principalRoles.principalId,
			to: r.principals.id
		}),
	},
	principals: {
		principalRoles: r.many.principalRoles(),
	},
}))