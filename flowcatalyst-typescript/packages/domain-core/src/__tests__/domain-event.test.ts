import { describe, it, expect } from 'vitest';
import { DomainEvent, BaseDomainEvent } from '../domain-event.js';
import { ExecutionContext } from '../execution-context.js';

describe('DomainEvent', () => {
	describe('generateId', () => {
		it('should generate unique IDs', () => {
			const ids = new Set<string>();
			for (let i = 0; i < 100; i++) {
				ids.add(DomainEvent.generateId());
			}
			expect(ids.size).toBe(100);
		});

		it('should generate 13-character TSID strings', () => {
			const id = DomainEvent.generateId();
			expect(id.length).toBe(13);
		});
	});

	describe('metadataFrom', () => {
		it('should create metadata from execution context', () => {
			const ctx: ExecutionContext = {
				executionId: 'exec-123',
				correlationId: 'corr-456',
				causationId: 'cause-789',
				principalId: 'principal-abc',
				initiatedAt: new Date(),
			};

			const metadata = DomainEvent.metadataFrom(ctx);

			expect(metadata.eventId.length).toBe(13);
			expect(metadata.executionId).toBe('exec-123');
			expect(metadata.correlationId).toBe('corr-456');
			expect(metadata.causationId).toBe('cause-789');
			expect(metadata.principalId).toBe('principal-abc');
			expect(metadata.time).toBeInstanceOf(Date);
		});
	});

	describe('subject', () => {
		it('should create subject string', () => {
			const subject = DomainEvent.subject('platform', 'eventtype', '0HZXEQ5Y8JY5Z');
			expect(subject).toBe('platform.eventtype.0HZXEQ5Y8JY5Z');
		});
	});

	describe('messageGroup', () => {
		it('should create message group string', () => {
			const group = DomainEvent.messageGroup('platform', 'eventtype', '0HZXEQ5Y8JY5Z');
			expect(group).toBe('platform:eventtype:0HZXEQ5Y8JY5Z');
		});
	});

	describe('eventType', () => {
		it('should create event type code', () => {
			const type = DomainEvent.eventType('platform', 'control-plane', 'eventtype', 'created');
			expect(type).toBe('platform:control-plane:eventtype:created');
		});
	});

	describe('extractAggregateType', () => {
		it('should extract aggregate type from subject', () => {
			expect(DomainEvent.extractAggregateType('platform.eventtype.123456789')).toBe('Eventtype');
			expect(DomainEvent.extractAggregateType('platform.user-account.abc')).toBe('Useraccount');
		});

		it('should return Unknown for invalid subjects', () => {
			expect(DomainEvent.extractAggregateType('')).toBe('Unknown');
			expect(DomainEvent.extractAggregateType('single')).toBe('Unknown');
		});

		it('should handle null/undefined', () => {
			expect(DomainEvent.extractAggregateType(null as never)).toBe('Unknown');
		});
	});

	describe('extractEntityId', () => {
		it('should extract entity ID from subject', () => {
			expect(DomainEvent.extractEntityId('platform.eventtype.123456789')).toBe('123456789');
		});

		it('should return null for invalid subjects', () => {
			expect(DomainEvent.extractEntityId('')).toBeNull();
			expect(DomainEvent.extractEntityId('platform.eventtype')).toBeNull();
		});

		it('should handle null/undefined', () => {
			expect(DomainEvent.extractEntityId(null as never)).toBeNull();
		});
	});
});

describe('BaseDomainEvent', () => {
	interface TestEventData {
		userId: string;
		name: string;
	}

	class TestEvent extends BaseDomainEvent<TestEventData> {
		constructor(ctx: ExecutionContext, data: TestEventData) {
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

	it('should create event with all CloudEvents fields', () => {
		const ctx: ExecutionContext = {
			executionId: 'exec-test',
			correlationId: 'corr-test',
			causationId: null,
			principalId: 'principal-test',
			initiatedAt: new Date(),
		};

		const event = new TestEvent(ctx, { userId: 'user-123', name: 'Test User' });

		expect(event.eventId.length).toBe(13);
		expect(event.eventType).toBe('test:domain:user:created');
		expect(event.specVersion).toBe('1.0');
		expect(event.source).toBe('test:domain');
		expect(event.subject).toBe('test.user.user-123');
		expect(event.messageGroup).toBe('test:user:user-123');
		expect(event.executionId).toBe('exec-test');
		expect(event.correlationId).toBe('corr-test');
		expect(event.causationId).toBeNull();
		expect(event.principalId).toBe('principal-test');
		expect(event.time).toBeInstanceOf(Date);
	});

	it('should serialize data to JSON', () => {
		const ctx: ExecutionContext = {
			executionId: 'exec',
			correlationId: 'corr',
			causationId: null,
			principalId: 'principal',
			initiatedAt: new Date(),
		};

		const event = new TestEvent(ctx, { userId: 'user-456', name: 'Another User' });
		const json = event.toDataJson();

		expect(JSON.parse(json)).toEqual({ userId: 'user-456', name: 'Another User' });
	});

	it('should provide access to event data', () => {
		const ctx: ExecutionContext = {
			executionId: 'exec',
			correlationId: 'corr',
			causationId: null,
			principalId: 'principal',
			initiatedAt: new Date(),
		};

		const event = new TestEvent(ctx, { userId: 'user-789', name: 'Data User' });
		const data = event.getData();

		expect(data.userId).toBe('user-789');
		expect(data.name).toBe('Data User');
	});
});
