/**
 * Anchor Domain Entity
 *
 * Represents an email domain that grants ANCHOR (platform admin) scope to users.
 * Users with email addresses from anchor domains automatically get ANCHOR scope.
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Anchor domain entity.
 */
export interface AnchorDomain {
	/** TSID primary key */
	readonly id: string;

	/** The domain (e.g., "flowcatalyst.tech") */
	readonly domain: string;

	/** When the anchor domain was created */
	readonly createdAt: Date;

	/** When the anchor domain was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new AnchorDomain.
 */
export type NewAnchorDomain = Omit<AnchorDomain, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new anchor domain.
 */
export function createAnchorDomain(domain: string): NewAnchorDomain {
	return {
		id: generate('ANCHOR_DOMAIN'),
		domain: domain.toLowerCase(),
	};
}
