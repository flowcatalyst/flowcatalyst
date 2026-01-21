import { describe, it, expect } from 'vitest';
import { Hono } from 'hono';
import { Result, UseCaseError } from '@flowcatalyst/domain-core';
import {
	getErrorStatus,
	toErrorResponse,
	sendResult,
	matchResult,
	jsonSuccess,
	jsonCreated,
	noContent,
	notFound,
	unauthorized,
	forbidden,
	badRequest,
} from '../response.js';
import type { FlowCatalystEnv } from '../types.js';

describe('Response Utilities', () => {
	describe('getErrorStatus', () => {
		it('should return 400 for validation errors', () => {
			const error = UseCaseError.validation('CODE', 'message');
			expect(getErrorStatus(error)).toBe(400);
		});

		it('should return 404 for not found errors', () => {
			const error = UseCaseError.notFound('CODE', 'message');
			expect(getErrorStatus(error)).toBe(404);
		});

		it('should return 409 for business rule violations', () => {
			const error = UseCaseError.businessRule('CODE', 'message');
			expect(getErrorStatus(error)).toBe(409);
		});

		it('should return 409 for concurrency errors', () => {
			const error = UseCaseError.concurrency('CODE', 'message');
			expect(getErrorStatus(error)).toBe(409);
		});
	});

	describe('toErrorResponse', () => {
		it('should create error response with details', () => {
			const error = UseCaseError.validation('INVALID_EMAIL', 'Invalid email', { field: 'email' });
			const response = toErrorResponse(error);

			expect(response.code).toBe('INVALID_EMAIL');
			expect(response.message).toBe('Invalid email');
			expect(response.details).toEqual({ field: 'email' });
		});

		it('should omit empty details', () => {
			const error = UseCaseError.validation('CODE', 'message');
			const response = toErrorResponse(error);

			expect(response.details).toBeUndefined();
		});
	});

	describe('sendResult', () => {
		it('should send success with default status', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				// Create a simple success result for testing
				const result: Result<{ id: string }> = { _tag: 'success', value: { id: '123' } };
				return sendResult(c, result);
			});

			const res = await app.request('/test');
			expect(res.status).toBe(200);
			expect(await res.json()).toEqual({ id: '123' });
		});

		it('should send success with custom status', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				const result: Result<{ id: string }> = { _tag: 'success', value: { id: '456' } };
				return sendResult(c, result, { successStatus: 201 });
			});

			const res = await app.request('/test');
			expect(res.status).toBe(201);
		});

		it('should transform success value', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				const result: Result<{ userId: string; name: string }> = {
					_tag: 'success',
					value: { userId: '789', name: 'Test' },
				};
				return sendResult(c, result, {
					transform: (v) => ({ id: v.userId }),
				});
			});

			const res = await app.request('/test');
			expect(await res.json()).toEqual({ id: '789' });
		});

		it('should send failure with appropriate status', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				const result = Result.failure(UseCaseError.notFound('NOT_FOUND', 'User not found'));
				return sendResult(c, result);
			});

			const res = await app.request('/test');
			expect(res.status).toBe(404);
			const data = await res.json();
			expect(data.code).toBe('NOT_FOUND');
		});
	});

	describe('matchResult', () => {
		it('should call onSuccess for success result', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				const result: Result<string> = { _tag: 'success', value: 'hello' };
				return matchResult(
					c,
					result,
					(value) => c.json({ greeting: value }),
				);
			});

			const res = await app.request('/test');
			expect(await res.json()).toEqual({ greeting: 'hello' });
		});

		it('should call onFailure for failure result', async () => {
			const app = new Hono<FlowCatalystEnv>();
			app.get('/test', (c) => {
				const result = Result.failure(UseCaseError.validation('ERR', 'error'));
				return matchResult(
					c,
					result,
					() => c.json({ ok: true }),
					(error) => c.json({ custom: error.code }, 400),
				);
			});

			const res = await app.request('/test');
			expect(res.status).toBe(400);
			expect(await res.json()).toEqual({ custom: 'ERR' });
		});
	});

	describe('Response helpers', () => {
		it('jsonSuccess should return 200 by default', async () => {
			const app = new Hono();
			app.get('/test', (c) => jsonSuccess(c, { ok: true }));

			const res = await app.request('/test');
			expect(res.status).toBe(200);
		});

		it('jsonCreated should return 201', async () => {
			const app = new Hono();
			app.get('/test', (c) => jsonCreated(c, { id: '123' }));

			const res = await app.request('/test');
			expect(res.status).toBe(201);
		});

		it('noContent should return 204', async () => {
			const app = new Hono();
			app.get('/test', (c) => noContent(c));

			const res = await app.request('/test');
			expect(res.status).toBe(204);
		});

		it('notFound should return 404', async () => {
			const app = new Hono();
			app.get('/test', (c) => notFound(c));

			const res = await app.request('/test');
			expect(res.status).toBe(404);
			expect((await res.json()).code).toBe('NOT_FOUND');
		});

		it('unauthorized should return 401', async () => {
			const app = new Hono();
			app.get('/test', (c) => unauthorized(c));

			const res = await app.request('/test');
			expect(res.status).toBe(401);
		});

		it('forbidden should return 403', async () => {
			const app = new Hono();
			app.get('/test', (c) => forbidden(c));

			const res = await app.request('/test');
			expect(res.status).toBe(403);
		});

		it('badRequest should return 400 with details', async () => {
			const app = new Hono();
			app.get('/test', (c) => badRequest(c, 'Invalid input', { field: 'email' }));

			const res = await app.request('/test');
			expect(res.status).toBe(400);
			const data = await res.json();
			expect(data.message).toBe('Invalid input');
			expect(data.details).toEqual({ field: 'email' });
		});
	});
});
