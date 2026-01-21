import { describe, it, expect } from 'vitest';
import { createPagedResult } from '../repository.js';

describe('Repository', () => {
	describe('createPagedResult', () => {
		it('should create paged result with correct metadata', () => {
			const items = [{ id: '1' }, { id: '2' }, { id: '3' }];
			const result = createPagedResult(items, 0, 10, 25);

			expect(result.items).toEqual(items);
			expect(result.page).toBe(0);
			expect(result.pageSize).toBe(10);
			expect(result.totalItems).toBe(25);
			expect(result.totalPages).toBe(3);
			expect(result.hasNext).toBe(true);
			expect(result.hasPrevious).toBe(false);
		});

		it('should indicate hasNext correctly', () => {
			const items = [{ id: '1' }];

			// First page of 3
			const first = createPagedResult(items, 0, 10, 30);
			expect(first.hasNext).toBe(true);
			expect(first.hasPrevious).toBe(false);

			// Middle page
			const middle = createPagedResult(items, 1, 10, 30);
			expect(middle.hasNext).toBe(true);
			expect(middle.hasPrevious).toBe(true);

			// Last page
			const last = createPagedResult(items, 2, 10, 30);
			expect(last.hasNext).toBe(false);
			expect(last.hasPrevious).toBe(true);
		});

		it('should calculate totalPages correctly', () => {
			// Exact division
			expect(createPagedResult([], 0, 10, 100).totalPages).toBe(10);

			// With remainder
			expect(createPagedResult([], 0, 10, 101).totalPages).toBe(11);
			expect(createPagedResult([], 0, 10, 5).totalPages).toBe(1);

			// Empty
			expect(createPagedResult([], 0, 10, 0).totalPages).toBe(0);
		});

		it('should handle single page', () => {
			const items = [{ id: '1' }, { id: '2' }];
			const result = createPagedResult(items, 0, 10, 2);

			expect(result.totalPages).toBe(1);
			expect(result.hasNext).toBe(false);
			expect(result.hasPrevious).toBe(false);
		});
	});
});
