/**
 * Assign Application Access Command
 */

export interface AssignApplicationAccessCommand {
  readonly _type?: string;
  readonly userId: string;
  readonly applicationIds: string[];
}
