/**
 * SpecVersion Value Object
 *
 * Represents a schema version for an event type.
 * Version format is "MAJOR.MINOR" (e.g., "1.0", "2.1").
 * Schema content is stored as a JSON Schema object (jsonb).
 */

import { generate } from '@flowcatalyst/tsid';
import type { SchemaType } from './schema-type.js';
import type { SpecVersionStatus } from './spec-version-status.js';

/**
 * SpecVersion entity.
 */
export interface SpecVersion {
  readonly id: string;
  readonly eventTypeId: string;
  readonly version: string;
  readonly mimeType: string;
  readonly schemaContent: unknown | null;
  readonly schemaType: SchemaType;
  readonly status: SpecVersionStatus;
  readonly createdAt: Date;
  readonly updatedAt: Date;
}

/**
 * Input for creating a new SpecVersion.
 */
export type NewSpecVersion = Omit<SpecVersion, 'createdAt' | 'updatedAt'> & {
  createdAt?: Date;
  updatedAt?: Date;
};

/**
 * Create a new spec version in FINALISING status.
 */
export function createSpecVersion(params: {
  eventTypeId: string;
  version: string;
  mimeType: string;
  schemaContent: unknown | null;
  schemaType: SchemaType;
}): NewSpecVersion {
  return {
    id: generate('SCHEMA'),
    eventTypeId: params.eventTypeId,
    version: params.version,
    mimeType: params.mimeType,
    schemaContent: params.schemaContent,
    schemaType: params.schemaType,
    status: 'FINALISING',
  };
}

/**
 * Extract the major version number from a version string.
 */
export function majorVersion(version: string): number {
  return parseInt(version.split('.')[0]!, 10);
}

/**
 * Extract the minor version number from a version string.
 */
export function minorVersion(version: string): number {
  return parseInt(version.split('.')[1]!, 10);
}

/**
 * Create a copy of a spec version with a new status.
 */
export function withStatus(specVersion: SpecVersion, status: SpecVersionStatus): SpecVersion {
  return {
    ...specVersion,
    status,
    updatedAt: new Date(),
  };
}
