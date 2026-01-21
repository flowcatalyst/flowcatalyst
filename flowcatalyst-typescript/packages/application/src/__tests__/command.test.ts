import { describe, it, expect } from 'vitest';
import { createCommand, type Command, type EntityCommand, type DeleteCommand } from '../command.js';

describe('Command', () => {
	describe('createCommand', () => {
		it('should create command with _type field', () => {
			const command = createCommand('CreateUser', {
				email: 'user@example.com',
				name: 'Test User',
			});

			expect(command._type).toBe('CreateUser');
			expect(command.email).toBe('user@example.com');
			expect(command.name).toBe('Test User');
		});

		it('should merge _type with command data', () => {
			const command = createCommand('UpdateUser', {
				userId: 'user-123',
				name: 'Updated Name',
			});

			expect(command._type).toBe('UpdateUser');
			expect(command.userId).toBe('user-123');
			expect(command.name).toBe('Updated Name');
		});
	});

	describe('Command type', () => {
		it('should allow commands without _type', () => {
			interface SimpleCommand extends Command {
				value: string;
			}

			const cmd: SimpleCommand = { value: 'test' };
			expect(cmd._type).toBeUndefined();
			expect(cmd.value).toBe('test');
		});

		it('should allow commands with _type', () => {
			interface TypedCommand extends Command {
				_type: 'MyCommand';
				value: string;
			}

			const cmd: TypedCommand = { _type: 'MyCommand', value: 'test' };
			expect(cmd._type).toBe('MyCommand');
		});
	});

	describe('EntityCommand type', () => {
		it('should require entity ID field', () => {
			type UpdateUserCommand = EntityCommand<{ name?: string }, 'userId'>;

			const cmd: UpdateUserCommand = { userId: 'user-123', name: 'New Name' };
			expect(cmd.userId).toBe('user-123');
			expect(cmd.name).toBe('New Name');
		});

		it('should default to id field', () => {
			type UpdateEntityCommand = EntityCommand<{ value: number }>;

			const cmd: UpdateEntityCommand = { id: 'entity-456', value: 42 };
			expect(cmd.id).toBe('entity-456');
		});
	});

	describe('DeleteCommand type', () => {
		it('should have only ID field', () => {
			type DeleteUserCommand = DeleteCommand<'userId'>;

			const cmd: DeleteUserCommand = { userId: 'user-789' };
			expect(cmd.userId).toBe('user-789');
		});

		it('should default to id field', () => {
			type DeleteEntityCommand = DeleteCommand;

			const cmd: DeleteEntityCommand = { id: 'entity-000' };
			expect(cmd.id).toBe('entity-000');
		});
	});
});
