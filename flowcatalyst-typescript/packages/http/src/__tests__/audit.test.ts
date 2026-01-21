import { describe, it, expect, vi, beforeEach } from 'vitest';
import { Hono } from 'hono';
import { auditMiddleware, requireAuth, isAuthenticated, getPrincipalId } from '../middleware/audit.js';
import type { FlowCatalystEnv } from '../types.js';

describe('Audit Middleware', () => {
	const mockValidateToken = vi.fn();

	beforeEach(() => {
		mockValidateToken.mockReset();
	});

	it('should authenticate with Bearer token', async () => {
		mockValidateToken.mockResolvedValue('user-123');

		const app = new Hono<FlowCatalystEnv>();
		app.use('*', auditMiddleware({ validateToken: mockValidateToken }));
		app.get('/test', (c) => {
			const audit = c.get('audit');
			return c.json({ principalId: audit.principalId });
		});

		const res = await app.request('/test', {
			headers: { Authorization: 'Bearer valid-token' },
		});
		const data = await res.json();

		expect(data.principalId).toBe('user-123');
		expect(mockValidateToken).toHaveBeenCalledWith('valid-token');
	});

	it('should authenticate with session cookie', async () => {
		mockValidateToken.mockResolvedValue('user-456');

		const app = new Hono<FlowCatalystEnv>();
		app.use('*', auditMiddleware({ validateToken: mockValidateToken }));
		app.get('/test', (c) => {
			const audit = c.get('audit');
			return c.json({ principalId: audit.principalId });
		});

		const res = await app.request('/test', {
			headers: { Cookie: 'session=session-token' },
		});
		const data = await res.json();

		expect(data.principalId).toBe('user-456');
		expect(mockValidateToken).toHaveBeenCalledWith('session-token');
	});

	it('should set null when not authenticated', async () => {
		mockValidateToken.mockResolvedValue(null);

		const app = new Hono<FlowCatalystEnv>();
		app.use('*', auditMiddleware({ validateToken: mockValidateToken }));
		app.get('/test', (c) => {
			const audit = c.get('audit');
			return c.json({ principalId: audit.principalId });
		});

		const res = await app.request('/test');
		const data = await res.json();

		expect(data.principalId).toBeNull();
	});

	it('should skip paths', async () => {
		mockValidateToken.mockResolvedValue('user-789');

		const app = new Hono<FlowCatalystEnv>();
		app.use(
			'*',
			auditMiddleware({
				validateToken: mockValidateToken,
				skipPaths: ['/health'],
			}),
		);
		app.get('/health', (c) => {
			const audit = c.get('audit');
			return c.json({ principalId: audit.principalId });
		});

		const res = await app.request('/health', {
			headers: { Authorization: 'Bearer token' },
		});
		const data = await res.json();

		expect(data.principalId).toBeNull();
		expect(mockValidateToken).not.toHaveBeenCalled();
	});

	it('should load principal when configured', async () => {
		mockValidateToken.mockResolvedValue('user-loaded');
		const mockLoadPrincipal = vi.fn().mockResolvedValue({
			id: 'user-loaded',
			name: 'Test User',
			type: 'USER',
			scope: 'CLIENT',
			roles: new Set(['admin']),
		});

		const app = new Hono<FlowCatalystEnv>();
		app.use(
			'*',
			auditMiddleware({
				validateToken: mockValidateToken,
				loadPrincipal: mockLoadPrincipal,
			}),
		);
		app.get('/test', (c) => {
			const audit = c.get('audit');
			return c.json({
				principalId: audit.principalId,
				principalName: audit.principal?.name,
			});
		});

		const res = await app.request('/test', {
			headers: { Authorization: 'Bearer token' },
		});
		const data = await res.json();

		expect(data.principalId).toBe('user-loaded');
		expect(data.principalName).toBe('Test User');
	});

	describe('requireAuth', () => {
		it('should return principal ID when authenticated', () => {
			const mockContext = {
				get: () => ({ principalId: 'auth-user', principal: null }),
			};
			expect(requireAuth(mockContext as never)).toBe('auth-user');
		});

		it('should throw 401 when not authenticated', () => {
			const mockContext = { get: () => ({ principalId: null, principal: null }) };
			expect(() => requireAuth(mockContext as never)).toThrow();
		});
	});

	describe('isAuthenticated', () => {
		it('should return true when authenticated', () => {
			const mockContext = {
				get: () => ({ principalId: 'user', principal: null }),
			};
			expect(isAuthenticated(mockContext as never)).toBe(true);
		});

		it('should return false when not authenticated', () => {
			const mockContext = { get: () => ({ principalId: null, principal: null }) };
			expect(isAuthenticated(mockContext as never)).toBe(false);
		});

		it('should return false when audit not set', () => {
			const mockContext = { get: () => undefined };
			expect(isAuthenticated(mockContext as never)).toBe(false);
		});
	});

	describe('getPrincipalId', () => {
		it('should return principal ID', () => {
			const mockContext = {
				get: () => ({ principalId: 'get-user', principal: null }),
			};
			expect(getPrincipalId(mockContext as never)).toBe('get-user');
		});

		it('should return null when not set', () => {
			const mockContext = { get: () => undefined };
			expect(getPrincipalId(mockContext as never)).toBeNull();
		});
	});
});
