/**
 * Client Access Grant
 *
 * Represents a grant of access to a specific client for a user.
 * Used for PARTNER scope users who need explicit access to specific clients.
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Client access grant entity.
 */
export interface ClientAccessGrant {
	/** TSID primary key */
	readonly id: string;

	/** Principal (user) who is granted access */
	readonly principalId: string;

	/** Client the user is granted access to */
	readonly clientId: string;

	/** Who granted this access */
	readonly grantedBy: string;

	/** When access was granted */
	readonly grantedAt: Date;

	/** When the record was created */
	readonly createdAt: Date;

	/** When the record was last updated */
	readonly updatedAt: Date;
}

/**
 * Type for creating a new ClientAccessGrant.
 */
export type NewClientAccessGrant = Omit<ClientAccessGrant, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new client access grant.
 */
export function createClientAccessGrant(params: {
	principalId: string;
	clientId: string;
	grantedBy: string;
}): NewClientAccessGrant {
	const now = new Date();
	return {
		id: generate('CLIENT_ACCESS_GRANT'),
		principalId: params.principalId,
		clientId: params.clientId,
		grantedBy: params.grantedBy,
		grantedAt: now,
	};
}
