import { describe, it, expect } from 'vitest';
import { AuditContext } from '../audit-context.js';

describe('AuditContext', () => {
	describe('outside context', () => {
		it('should return null for getPrincipal', () => {
			expect(AuditContext.getPrincipal()).toBeNull();
		});

		it('should return null for getPrincipalId', () => {
			expect(AuditContext.getPrincipalId()).toBeNull();
		});

		it('should return false for isAuthenticated', () => {
			expect(AuditContext.isAuthenticated()).toBe(false);
		});

		it('should throw from requirePrincipalId', () => {
			expect(() => AuditContext.requirePrincipalId()).toThrow('Authentication required');
		});

		it('should throw from requirePrincipal', () => {
			expect(() => AuditContext.requirePrincipal()).toThrow('Authentication required');
		});
	});

	describe('runWithPrincipal', () => {
		const testPrincipal = AuditContext.createPrincipal('principal-123', 'USER', 'CLIENT', 'client-456', [
			'admin',
			'viewer',
		]);

		it('should provide principal within context', () => {
			AuditContext.runWithPrincipal(testPrincipal, () => {
				expect(AuditContext.getPrincipal()).toEqual(testPrincipal);
				expect(AuditContext.getPrincipalId()).toBe('principal-123');
			});
		});

		it('should indicate authenticated within context', () => {
			AuditContext.runWithPrincipal(testPrincipal, () => {
				expect(AuditContext.isAuthenticated()).toBe(true);
			});
		});

		it('should return function result', () => {
			const result = AuditContext.runWithPrincipal(testPrincipal, () => 'result');
			expect(result).toBe('result');
		});

		it('should not leak context outside', () => {
			AuditContext.runWithPrincipal(testPrincipal, () => {
				expect(AuditContext.isAuthenticated()).toBe(true);
			});
			expect(AuditContext.isAuthenticated()).toBe(false);
		});
	});

	describe('runWithPrincipalAsync', () => {
		const testPrincipal = AuditContext.createPrincipal('principal-async', 'USER', 'ANCHOR', null, ['admin']);

		it('should work with async functions', async () => {
			const result = await AuditContext.runWithPrincipalAsync(testPrincipal, async () => {
				await new Promise((r) => setTimeout(r, 10));
				return AuditContext.getPrincipalId();
			});

			expect(result).toBe('principal-async');
		});
	});

	describe('authorization checks', () => {
		describe('hasAccessToAllClients', () => {
			it('should return true for ANCHOR scope', () => {
				const anchor = AuditContext.createPrincipal('p1', 'USER', 'ANCHOR', null, []);
				AuditContext.runWithPrincipal(anchor, () => {
					expect(AuditContext.hasAccessToAllClients()).toBe(true);
				});
			});

			it('should return false for CLIENT scope', () => {
				const client = AuditContext.createPrincipal('p2', 'USER', 'CLIENT', 'client-1', []);
				AuditContext.runWithPrincipal(client, () => {
					expect(AuditContext.hasAccessToAllClients()).toBe(false);
				});
			});

			it('should return false for PARTNER scope', () => {
				const partner = AuditContext.createPrincipal('p3', 'USER', 'PARTNER', 'client-1', []);
				AuditContext.runWithPrincipal(partner, () => {
					expect(AuditContext.hasAccessToAllClients()).toBe(false);
				});
			});
		});

		describe('hasAccessToClient', () => {
			it('should always allow ANCHOR to access any client', () => {
				const anchor = AuditContext.createPrincipal('p1', 'USER', 'ANCHOR', null, []);
				AuditContext.runWithPrincipal(anchor, () => {
					expect(AuditContext.hasAccessToClient('any-client')).toBe(true);
					expect(AuditContext.hasAccessToClient('another-client')).toBe(true);
				});
			});

			it('should allow CLIENT to access their home client', () => {
				const client = AuditContext.createPrincipal('p2', 'USER', 'CLIENT', 'home-client', []);
				AuditContext.runWithPrincipal(client, () => {
					expect(AuditContext.hasAccessToClient('home-client')).toBe(true);
				});
			});

			it('should deny CLIENT access to other clients', () => {
				const client = AuditContext.createPrincipal('p2', 'USER', 'CLIENT', 'home-client', []);
				AuditContext.runWithPrincipal(client, () => {
					expect(AuditContext.hasAccessToClient('other-client')).toBe(false);
				});
			});

			it('should return false when not authenticated', () => {
				expect(AuditContext.hasAccessToClient('any-client')).toBe(false);
			});
		});

		describe('hasRole', () => {
			it('should return true when principal has role', () => {
				const principal = AuditContext.createPrincipal('p1', 'USER', 'CLIENT', 'c1', ['admin', 'viewer']);
				AuditContext.runWithPrincipal(principal, () => {
					expect(AuditContext.hasRole('admin')).toBe(true);
					expect(AuditContext.hasRole('viewer')).toBe(true);
				});
			});

			it('should return false when principal lacks role', () => {
				const principal = AuditContext.createPrincipal('p1', 'USER', 'CLIENT', 'c1', ['viewer']);
				AuditContext.runWithPrincipal(principal, () => {
					expect(AuditContext.hasRole('admin')).toBe(false);
				});
			});

			it('should return false when not authenticated', () => {
				expect(AuditContext.hasRole('admin')).toBe(false);
			});
		});

		describe('getRoles', () => {
			it('should return roles as ReadonlySet', () => {
				const principal = AuditContext.createPrincipal('p1', 'USER', 'CLIENT', 'c1', ['admin', 'viewer']);
				AuditContext.runWithPrincipal(principal, () => {
					const roles = AuditContext.getRoles();
					expect(roles.has('admin')).toBe(true);
					expect(roles.has('viewer')).toBe(true);
					expect(roles.size).toBe(2);
				});
			});

			it('should return empty set when not authenticated', () => {
				const roles = AuditContext.getRoles();
				expect(roles.size).toBe(0);
			});
		});

		describe('getHomeClientId', () => {
			it('should return home client ID', () => {
				const principal = AuditContext.createPrincipal('p1', 'USER', 'CLIENT', 'home-123', []);
				AuditContext.runWithPrincipal(principal, () => {
					expect(AuditContext.getHomeClientId()).toBe('home-123');
				});
			});

			it('should return null for ANCHOR without home client', () => {
				const anchor = AuditContext.createPrincipal('p1', 'USER', 'ANCHOR', null, []);
				AuditContext.runWithPrincipal(anchor, () => {
					expect(AuditContext.getHomeClientId()).toBeNull();
				});
			});
		});
	});

	describe('runAsSystem', () => {
		it('should run as SYSTEM principal', () => {
			AuditContext.runAsSystem('system-id', () => {
				expect(AuditContext.getPrincipalId()).toBe('system-id');
				expect(AuditContext.isSystemPrincipal()).toBe(true);
				expect(AuditContext.hasAccessToAllClients()).toBe(true);
			});
		});

		it('should have SYSTEM role', () => {
			AuditContext.runAsSystem('system-id', () => {
				expect(AuditContext.hasRole('SYSTEM')).toBe(true);
			});
		});
	});

	describe('createPrincipal', () => {
		it('should create principal with all fields', () => {
			const principal = AuditContext.createPrincipal('id-1', 'SERVICE', 'PARTNER', 'client-x', ['role1', 'role2']);

			expect(principal.id).toBe('id-1');
			expect(principal.type).toBe('SERVICE');
			expect(principal.scope).toBe('PARTNER');
			expect(principal.clientId).toBe('client-x');
			expect(principal.roles.has('role1')).toBe(true);
			expect(principal.roles.has('role2')).toBe(true);
		});

		it('should default to empty roles', () => {
			const principal = AuditContext.createPrincipal('id-1', 'USER', 'CLIENT', 'c1');
			expect(principal.roles.size).toBe(0);
		});
	});
});
