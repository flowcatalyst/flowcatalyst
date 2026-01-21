import { describe, it, expect } from 'vitest';
import {
	Result,
	UseCaseError,
	ExecutionContext,
	BaseDomainEvent,
	DomainEvent,
	RESULT_SUCCESS_TOKEN,
	type UnitOfWork,
	type Aggregate,
} from '@flowcatalyst/domain-core';
import type { UseCase } from '../use-case.js';
import type { Command } from '../command.js';
import { validateRequired, validateEmail } from '../validation.js';

// Test domain types
interface CreateUserCommand extends Command {
	email: string;
	name: string;
}

interface UserCreatedData {
	userId: string;
	email: string;
	name: string;
}

class UserCreatedEvent extends BaseDomainEvent<UserCreatedData> {
	constructor(ctx: ExecutionContext, data: UserCreatedData) {
		super(
			{
				eventType: 'test:domain:user:created',
				specVersion: '1.0',
				source: 'test:domain',
				subject: DomainEvent.subject('test', 'user', data.userId),
				messageGroup: DomainEvent.messageGroup('test', 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

interface User {
	id: string;
	email: string;
	name: string;
}

interface UserRepository {
	findByEmail(email: string): Promise<User | null>;
}

describe('UseCase Pattern', () => {
	const ctx = ExecutionContext.create('test-principal');

	// Example use case implementation
	class CreateUserUseCase implements UseCase<CreateUserCommand, UserCreatedEvent> {
		constructor(
			private userRepo: UserRepository,
			private unitOfWork: UnitOfWork,
		) {}

		async execute(command: CreateUserCommand, context: ExecutionContext): Promise<Result<UserCreatedEvent>> {
			// 1. Validation
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) return nameResult as Result<UserCreatedEvent>;

			const emailResult = validateEmail(command.email);
			if (Result.isFailure(emailResult)) return emailResult as Result<UserCreatedEvent>;

			// 2. Business rules
			const existing = await this.userRepo.findByEmail(command.email);
			if (existing) {
				return Result.failure(
					UseCaseError.businessRule('EMAIL_EXISTS', 'Email is already registered', {
						email: command.email,
					}),
				);
			}

			// 3. Create aggregate
			const user: User = {
				id: 'user-123',
				email: command.email,
				name: command.name,
			};

			// 4. Create domain event
			const event = new UserCreatedEvent(context, {
				userId: user.id,
				email: user.email,
				name: user.name,
			});

			// 5. Commit (only way to return success)
			return this.unitOfWork.commit(user, event, command);
		}
	}

	it('should return success when valid command', async () => {
		const mockRepo: UserRepository = {
			findByEmail: async () => null,
		};

		const mockUnitOfWork: UnitOfWork = {
			commit: async <T extends DomainEvent>(_agg: Aggregate, event: T) =>
				Result.success(RESULT_SUCCESS_TOKEN, event),
			commitDelete: async <T extends DomainEvent>(_agg: Aggregate, event: T) =>
				Result.success(RESULT_SUCCESS_TOKEN, event),
			commitAll: async <T extends DomainEvent>(_aggs: Aggregate[], event: T) =>
				Result.success(RESULT_SUCCESS_TOKEN, event),
		};

		const useCase = new CreateUserUseCase(mockRepo, mockUnitOfWork);

		const result = await useCase.execute(
			{ email: 'user@example.com', name: 'Test User' },
			ctx,
		);

		expect(Result.isSuccess(result)).toBe(true);
		if (Result.isSuccess(result)) {
			expect(result.value.getData().email).toBe('user@example.com');
			expect(result.value.getData().name).toBe('Test User');
		}
	});

	it('should return validation error for missing name', async () => {
		const mockRepo: UserRepository = { findByEmail: async () => null };
		const mockUnitOfWork: UnitOfWork = {
			commit: async () => { throw new Error('Should not be called'); },
			commitDelete: async () => { throw new Error('Should not be called'); },
			commitAll: async () => { throw new Error('Should not be called'); },
		};

		const useCase = new CreateUserUseCase(mockRepo, mockUnitOfWork);

		const result = await useCase.execute(
			{ email: 'user@example.com', name: '' },
			ctx,
		);

		expect(Result.isFailure(result)).toBe(true);
		if (Result.isFailure(result)) {
			expect(result.error.type).toBe('validation');
			expect(result.error.code).toBe('NAME_REQUIRED');
		}
	});

	it('should return validation error for invalid email', async () => {
		const mockRepo: UserRepository = { findByEmail: async () => null };
		const mockUnitOfWork: UnitOfWork = {
			commit: async () => { throw new Error('Should not be called'); },
			commitDelete: async () => { throw new Error('Should not be called'); },
			commitAll: async () => { throw new Error('Should not be called'); },
		};

		const useCase = new CreateUserUseCase(mockRepo, mockUnitOfWork);

		const result = await useCase.execute(
			{ email: 'invalid-email', name: 'Test' },
			ctx,
		);

		expect(Result.isFailure(result)).toBe(true);
		if (Result.isFailure(result)) {
			expect(result.error.type).toBe('validation');
			expect(result.error.code).toBe('INVALID_EMAIL');
		}
	});

	it('should return business rule error for duplicate email', async () => {
		const mockRepo: UserRepository = {
			findByEmail: async () => ({ id: 'existing', email: 'user@example.com', name: 'Existing' }),
		};
		const mockUnitOfWork: UnitOfWork = {
			commit: async () => { throw new Error('Should not be called'); },
			commitDelete: async () => { throw new Error('Should not be called'); },
			commitAll: async () => { throw new Error('Should not be called'); },
		};

		const useCase = new CreateUserUseCase(mockRepo, mockUnitOfWork);

		const result = await useCase.execute(
			{ email: 'user@example.com', name: 'Test' },
			ctx,
		);

		expect(Result.isFailure(result)).toBe(true);
		if (Result.isFailure(result)) {
			expect(result.error.type).toBe('business_rule');
			expect(result.error.code).toBe('EMAIL_EXISTS');
		}
	});

	it('should pass command to UnitOfWork for audit', async () => {
		const mockRepo: UserRepository = { findByEmail: async () => null };

		let capturedCommand: unknown;
		const mockUnitOfWork: UnitOfWork = {
			commit: async <T extends DomainEvent>(_agg: Aggregate, event: T, command: unknown) => {
				capturedCommand = command;
				return Result.success(RESULT_SUCCESS_TOKEN, event);
			},
			commitDelete: async () => { throw new Error('Not used'); },
			commitAll: async () => { throw new Error('Not used'); },
		};

		const useCase = new CreateUserUseCase(mockRepo, mockUnitOfWork);
		const command: CreateUserCommand = { email: 'user@example.com', name: 'Test' };

		await useCase.execute(command, ctx);

		expect(capturedCommand).toBe(command);
	});
});
