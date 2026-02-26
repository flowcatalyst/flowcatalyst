/**
 * Add Schema Command
 */

import type { Command } from "@flowcatalyst/application";
import type { SchemaType } from "../../../domain/index.js";

export interface AddSchemaCommand extends Command {
	readonly eventTypeId: string;
	readonly version: string;
	readonly mimeType: string;
	readonly schemaContent: unknown;
	readonly schemaType: SchemaType;
}
