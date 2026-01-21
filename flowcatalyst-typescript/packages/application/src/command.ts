/**
 * Command Types
 *
 * Commands represent the input to write operations (mutations).
 * They are plain data objects that carry the intent and data for a use case.
 *
 * Conventions:
 * - Commands are named in imperative form: CreateUser, UpdateProduct, DeleteOrder
 * - Commands are immutable (readonly properties)
 * - Commands may have optional fields for partial updates
 * - Commands do not contain validation logic (validation is in use cases)
 *
 * @example
 * ```typescript
 * interface CreateUserCommand extends Command {
 *     readonly email: string;
 *     readonly password: string;
 *     readonly name: string;
 *     readonly clientId: string | null;
 * }
 *
 * interface UpdateUserCommand extends Command {
 *     readonly userId: string;
 *     readonly name?: string;        // undefined = no change
 *     readonly clientId?: string | null;  // null = clear, undefined = no change
 * }
 * ```
 */

/**
 * Base marker interface for commands.
 * All commands should extend this interface.
 *
 * Commands are the input to use cases that modify state.
 * Each command represents a single, atomic operation intent.
 */
export interface Command {
	/**
	 * Optional operation type identifier.
	 * If not provided, the command class/interface name is used for audit logs.
	 */
	readonly _type?: string;
}

/**
 * Type helper to make certain fields of a command optional.
 * Useful for creating update commands from create commands.
 *
 * @example
 * ```typescript
 * interface CreateUserCommand extends Command {
 *     email: string;
 *     password: string;
 *     name: string;
 * }
 *
 * // Make all fields optional except userId
 * type UpdateUserCommand = { userId: string } & PartialCommand<CreateUserCommand>;
 * ```
 */
export type PartialCommand<T extends Command> = {
	readonly [K in keyof T]?: T[K];
};

/**
 * Type helper for commands that operate on a specific entity.
 * Adds an entityId field to the command.
 *
 * @example
 * ```typescript
 * interface UpdateProductData {
 *     name?: string;
 *     price?: number;
 * }
 *
 * type UpdateProductCommand = EntityCommand<UpdateProductData, 'productId'>;
 * // Results in: { productId: string; name?: string; price?: number; }
 * ```
 */
export type EntityCommand<T, TIdField extends string = 'id'> = Command & {
	readonly [K in TIdField]: string;
} & T;

/**
 * Type helper for delete commands.
 * A delete command typically only needs the entity ID.
 *
 * @example
 * ```typescript
 * type DeleteUserCommand = DeleteCommand<'userId'>;
 * // Results in: { userId: string; _type?: string; }
 * ```
 */
export type DeleteCommand<TIdField extends string = 'id'> = Command & {
	readonly [K in TIdField]: string;
};

/**
 * Create a command with an explicit type identifier.
 * Useful when you want to control the operation name in audit logs.
 *
 * @param type - The operation type identifier
 * @param data - The command data
 * @returns Command with _type set
 *
 * @example
 * ```typescript
 * const command = createCommand('CreateUser', {
 *     email: 'user@example.com',
 *     name: 'John',
 * });
 * // command._type === 'CreateUser'
 * ```
 */
export function createCommand<T extends Record<string, unknown>>(type: string, data: T): Command & T {
	return { _type: type, ...data };
}
