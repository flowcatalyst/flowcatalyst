# TypeScript Domain-Driven Design Patterns

A structured approach to building business domain-oriented applications in TypeScript with strong type safety, discriminated unions, and functional error handling.

## Core Principles

1. **Make illegal states unrepresentable** - Use the type system to prevent invalid data
2. **Explicit error handling** - No thrown exceptions for business logic
3. **Immutable by default** - Use `readonly` and avoid mutations
4. **Discriminated unions** - Model domain variants explicitly
5. **Use cases as the application boundary** - Single entry point per operation

---

## 1. Result Type (Functional Error Handling)

```typescript
// core/result.ts

export type Result<T, E = Error> = Success<T> | Failure<E>;

export interface Success<T> {
  readonly _tag: 'Success';
  readonly value: T;
}

export interface Failure<E> {
  readonly _tag: 'Failure';
  readonly error: E;
}

export const success = <T>(value: T): Success<T> => ({
  _tag: 'Success',
  value,
});

export const failure = <E>(error: E): Failure<E> => ({
  _tag: 'Failure',
  error,
});

export const isSuccess = <T, E>(result: Result<T, E>): result is Success<T> =>
  result._tag === 'Success';

export const isFailure = <T, E>(result: Result<T, E>): result is Failure<E> =>
  result._tag === 'Failure';

// Chainable operations
export const map = <T, U, E>(
  result: Result<T, E>,
  fn: (value: T) => U
): Result<U, E> =>
  isSuccess(result) ? success(fn(result.value)) : result;

export const flatMap = <T, U, E>(
  result: Result<T, E>,
  fn: (value: T) => Result<U, E>
): Result<U, E> =>
  isSuccess(result) ? fn(result.value) : result;

export const mapError = <T, E, F>(
  result: Result<T, E>,
  fn: (error: E) => F
): Result<T, F> =>
  isFailure(result) ? failure(fn(result.error)) : result;

// Async variant
export type AsyncResult<T, E = Error> = Promise<Result<T, E>>;
```

---

## 2. Domain Errors (Discriminated Union)

```typescript
// domain/errors.ts

export type DomainError =
  | ValidationError
  | NotFoundError
  | ConflictError
  | AuthorizationError
  | BusinessRuleViolation;

export interface ValidationError {
  readonly _tag: 'ValidationError';
  readonly field: string;
  readonly message: string;
  readonly value?: unknown;
}

export interface NotFoundError {
  readonly _tag: 'NotFoundError';
  readonly entity: string;
  readonly id: string;
}

export interface ConflictError {
  readonly _tag: 'ConflictError';
  readonly entity: string;
  readonly reason: string;
}

export interface AuthorizationError {
  readonly _tag: 'AuthorizationError';
  readonly action: string;
  readonly resource: string;
  readonly reason?: string;
}

export interface BusinessRuleViolation {
  readonly _tag: 'BusinessRuleViolation';
  readonly rule: string;
  readonly message: string;
  readonly context?: Record<string, unknown>;
}

// Factory functions
export const validationError = (
  field: string,
  message: string,
  value?: unknown
): ValidationError => ({
  _tag: 'ValidationError',
  field,
  message,
  value,
});

export const notFoundError = (entity: string, id: string): NotFoundError => ({
  _tag: 'NotFoundError',
  entity,
  id,
});

export const conflictError = (entity: string, reason: string): ConflictError => ({
  _tag: 'ConflictError',
  entity,
  reason,
});

export const authorizationError = (
  action: string,
  resource: string,
  reason?: string
): AuthorizationError => ({
  _tag: 'AuthorizationError',
  action,
  resource,
  reason,
});

export const businessRuleViolation = (
  rule: string,
  message: string,
  context?: Record<string, unknown>
): BusinessRuleViolation => ({
  _tag: 'BusinessRuleViolation',
  rule,
  message,
  context,
});

// Type guards
export const isValidationError = (e: DomainError): e is ValidationError =>
  e._tag === 'ValidationError';

export const isNotFoundError = (e: DomainError): e is NotFoundError =>
  e._tag === 'NotFoundError';
```

---

## 3. Value Objects (Branded Types)

```typescript
// domain/value-objects.ts

// Branded types prevent mixing up primitives
declare const brand: unique symbol;
type Brand<T, B> = T & { readonly [brand]: B };

// Email
export type Email = Brand<string, 'Email'>;

export const createEmail = (value: string): Result<Email, ValidationError> => {
  const trimmed = value.trim().toLowerCase();
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  if (!emailRegex.test(trimmed)) {
    return failure(validationError('email', 'Invalid email format', value));
  }

  return success(trimmed as Email);
};

// Money (immutable value object)
export type Currency = 'USD' | 'EUR' | 'GBP';

export interface Money {
  readonly amount: number;
  readonly currency: Currency;
}

export const createMoney = (
  amount: number,
  currency: Currency
): Result<Money, ValidationError> => {
  if (!Number.isFinite(amount)) {
    return failure(validationError('amount', 'Amount must be a finite number', amount));
  }

  if (amount < 0) {
    return failure(validationError('amount', 'Amount cannot be negative', amount));
  }

  // Round to 2 decimal places
  const rounded = Math.round(amount * 100) / 100;

  return success({ amount: rounded, currency });
};

export const addMoney = (a: Money, b: Money): Result<Money, BusinessRuleViolation> => {
  if (a.currency !== b.currency) {
    return failure(businessRuleViolation(
      'CurrencyMismatch',
      `Cannot add ${a.currency} and ${b.currency}`
    ));
  }

  return success({
    amount: Math.round((a.amount + b.amount) * 100) / 100,
    currency: a.currency,
  });
};

// Entity ID
export type EntityId<T extends string> = Brand<string, T>;

export type OrderId = EntityId<'Order'>;
export type CustomerId = EntityId<'Customer'>;
export type ProductId = EntityId<'Product'>;

export const createEntityId = <T extends string>(
  value: string,
  _type: T
): EntityId<T> => value as EntityId<T>;
```

---

## 4. Domain Events (Discriminated Union)

```typescript
// domain/events.ts

export type DomainEvent =
  | OrderCreated
  | OrderItemAdded
  | OrderSubmitted
  | OrderCancelled
  | PaymentReceived
  | OrderShipped;

interface BaseDomainEvent {
  readonly eventId: string;
  readonly occurredAt: Date;
  readonly aggregateId: string;
  readonly aggregateVersion: number;
}

export interface OrderCreated extends BaseDomainEvent {
  readonly _tag: 'OrderCreated';
  readonly customerId: CustomerId;
  readonly currency: Currency;
}

export interface OrderItemAdded extends BaseDomainEvent {
  readonly _tag: 'OrderItemAdded';
  readonly productId: ProductId;
  readonly quantity: number;
  readonly unitPrice: Money;
}

export interface OrderSubmitted extends BaseDomainEvent {
  readonly _tag: 'OrderSubmitted';
  readonly totalAmount: Money;
  readonly itemCount: number;
}

export interface OrderCancelled extends BaseDomainEvent {
  readonly _tag: 'OrderCancelled';
  readonly reason: string;
  readonly cancelledBy: string;
}

export interface PaymentReceived extends BaseDomainEvent {
  readonly _tag: 'PaymentReceived';
  readonly paymentId: string;
  readonly amount: Money;
  readonly method: 'card' | 'bank_transfer' | 'crypto';
}

export interface OrderShipped extends BaseDomainEvent {
  readonly _tag: 'OrderShipped';
  readonly trackingNumber: string;
  readonly carrier: string;
  readonly estimatedDelivery: Date;
}

// Event factory with auto-generated metadata
let eventCounter = 0;
const createEventMeta = (aggregateId: string, version: number): BaseDomainEvent => ({
  eventId: `evt_${Date.now()}_${++eventCounter}`,
  occurredAt: new Date(),
  aggregateId,
  aggregateVersion: version,
});

export const orderCreated = (
  aggregateId: string,
  version: number,
  customerId: CustomerId,
  currency: Currency
): OrderCreated => ({
  ...createEventMeta(aggregateId, version),
  _tag: 'OrderCreated',
  customerId,
  currency,
});

// Pattern matching helper
export const matchEvent = <T>(
  event: DomainEvent,
  handlers: {
    OrderCreated: (e: OrderCreated) => T;
    OrderItemAdded: (e: OrderItemAdded) => T;
    OrderSubmitted: (e: OrderSubmitted) => T;
    OrderCancelled: (e: OrderCancelled) => T;
    PaymentReceived: (e: PaymentReceived) => T;
    OrderShipped: (e: OrderShipped) => T;
  }
): T => {
  switch (event._tag) {
    case 'OrderCreated': return handlers.OrderCreated(event);
    case 'OrderItemAdded': return handlers.OrderItemAdded(event);
    case 'OrderSubmitted': return handlers.OrderSubmitted(event);
    case 'OrderCancelled': return handlers.OrderCancelled(event);
    case 'PaymentReceived': return handlers.PaymentReceived(event);
    case 'OrderShipped': return handlers.OrderShipped(event);
  }
};
```

---

## 5. Aggregate (Order Example)

```typescript
// domain/aggregates/order.ts

export type OrderStatus = 'draft' | 'submitted' | 'paid' | 'shipped' | 'cancelled';

export interface OrderItem {
  readonly productId: ProductId;
  readonly quantity: number;
  readonly unitPrice: Money;
}

export interface Order {
  readonly id: OrderId;
  readonly customerId: CustomerId;
  readonly status: OrderStatus;
  readonly items: readonly OrderItem[];
  readonly currency: Currency;
  readonly version: number;
  readonly createdAt: Date;
  readonly updatedAt: Date;
}

// Commands
export type OrderCommand =
  | CreateOrder
  | AddItem
  | RemoveItem
  | SubmitOrder
  | CancelOrder;

export interface CreateOrder {
  readonly _tag: 'CreateOrder';
  readonly orderId: OrderId;
  readonly customerId: CustomerId;
  readonly currency: Currency;
}

export interface AddItem {
  readonly _tag: 'AddItem';
  readonly productId: ProductId;
  readonly quantity: number;
  readonly unitPrice: Money;
}

export interface RemoveItem {
  readonly _tag: 'RemoveItem';
  readonly productId: ProductId;
}

export interface SubmitOrder {
  readonly _tag: 'SubmitOrder';
}

export interface CancelOrder {
  readonly _tag: 'CancelOrder';
  readonly reason: string;
  readonly cancelledBy: string;
}

// Command handler (pure function)
export const handleOrderCommand = (
  state: Order | null,
  command: OrderCommand
): Result<DomainEvent[], DomainError> => {
  switch (command._tag) {
    case 'CreateOrder':
      return handleCreateOrder(state, command);
    case 'AddItem':
      return handleAddItem(state, command);
    case 'RemoveItem':
      return handleRemoveItem(state, command);
    case 'SubmitOrder':
      return handleSubmitOrder(state);
    case 'CancelOrder':
      return handleCancelOrder(state, command);
  }
};

const handleCreateOrder = (
  state: Order | null,
  cmd: CreateOrder
): Result<DomainEvent[], DomainError> => {
  if (state !== null) {
    return failure(conflictError('Order', 'Order already exists'));
  }

  return success([
    orderCreated(cmd.orderId, 1, cmd.customerId, cmd.currency),
  ]);
};

const handleAddItem = (
  state: Order | null,
  cmd: AddItem
): Result<DomainEvent[], DomainError> => {
  if (state === null) {
    return failure(notFoundError('Order', 'unknown'));
  }

  if (state.status !== 'draft') {
    return failure(businessRuleViolation(
      'OrderNotEditable',
      'Cannot add items to a non-draft order',
      { status: state.status }
    ));
  }

  if (cmd.quantity <= 0) {
    return failure(validationError('quantity', 'Quantity must be positive', cmd.quantity));
  }

  if (cmd.unitPrice.currency !== state.currency) {
    return failure(businessRuleViolation(
      'CurrencyMismatch',
      `Item price currency ${cmd.unitPrice.currency} doesn't match order currency ${state.currency}`
    ));
  }

  return success([{
    ...createEventMeta(state.id, state.version + 1),
    _tag: 'OrderItemAdded',
    productId: cmd.productId,
    quantity: cmd.quantity,
    unitPrice: cmd.unitPrice,
  } as OrderItemAdded]);
};

const handleSubmitOrder = (
  state: Order | null
): Result<DomainEvent[], DomainError> => {
  if (state === null) {
    return failure(notFoundError('Order', 'unknown'));
  }

  if (state.status !== 'draft') {
    return failure(businessRuleViolation(
      'OrderNotDraft',
      'Only draft orders can be submitted',
      { status: state.status }
    ));
  }

  if (state.items.length === 0) {
    return failure(businessRuleViolation(
      'EmptyOrder',
      'Cannot submit an order with no items'
    ));
  }

  const totalAmount = calculateTotal(state);

  return success([{
    ...createEventMeta(state.id, state.version + 1),
    _tag: 'OrderSubmitted',
    totalAmount,
    itemCount: state.items.length,
  } as OrderSubmitted]);
};

const handleRemoveItem = (
  state: Order | null,
  cmd: RemoveItem
): Result<DomainEvent[], DomainError> => {
  // Implementation...
  return success([]);
};

const handleCancelOrder = (
  state: Order | null,
  cmd: CancelOrder
): Result<DomainEvent[], DomainError> => {
  if (state === null) {
    return failure(notFoundError('Order', 'unknown'));
  }

  if (state.status === 'shipped') {
    return failure(businessRuleViolation(
      'OrderAlreadyShipped',
      'Cannot cancel a shipped order'
    ));
  }

  if (state.status === 'cancelled') {
    return failure(businessRuleViolation(
      'OrderAlreadyCancelled',
      'Order is already cancelled'
    ));
  }

  return success([{
    ...createEventMeta(state.id, state.version + 1),
    _tag: 'OrderCancelled',
    reason: cmd.reason,
    cancelledBy: cmd.cancelledBy,
  } as OrderCancelled]);
};

// Event applicator (pure function)
export const applyOrderEvent = (state: Order | null, event: DomainEvent): Order => {
  switch (event._tag) {
    case 'OrderCreated':
      return {
        id: event.aggregateId as OrderId,
        customerId: event.customerId,
        status: 'draft',
        items: [],
        currency: event.currency,
        version: event.aggregateVersion,
        createdAt: event.occurredAt,
        updatedAt: event.occurredAt,
      };

    case 'OrderItemAdded':
      if (!state) throw new Error('Invalid state');
      return {
        ...state,
        items: [...state.items, {
          productId: event.productId,
          quantity: event.quantity,
          unitPrice: event.unitPrice,
        }],
        version: event.aggregateVersion,
        updatedAt: event.occurredAt,
      };

    case 'OrderSubmitted':
      if (!state) throw new Error('Invalid state');
      return {
        ...state,
        status: 'submitted',
        version: event.aggregateVersion,
        updatedAt: event.occurredAt,
      };

    case 'OrderCancelled':
      if (!state) throw new Error('Invalid state');
      return {
        ...state,
        status: 'cancelled',
        version: event.aggregateVersion,
        updatedAt: event.occurredAt,
      };

    default:
      return state!;
  }
};

// Helper
const calculateTotal = (order: Order): Money => {
  const total = order.items.reduce(
    (sum, item) => sum + item.unitPrice.amount * item.quantity,
    0
  );
  return { amount: Math.round(total * 100) / 100, currency: order.currency };
};
```

---

## 6. Use Case Pattern

```typescript
// application/use-cases/submit-order.ts

// Ports (interfaces for external dependencies)
export interface OrderRepository {
  findById(id: OrderId): AsyncResult<Order | null, DomainError>;
  save(order: Order, events: DomainEvent[]): AsyncResult<void, DomainError>;
}

export interface EventPublisher {
  publish(events: DomainEvent[]): AsyncResult<void, DomainError>;
}

export interface Logger {
  info(message: string, context?: Record<string, unknown>): void;
  error(message: string, error: unknown, context?: Record<string, unknown>): void;
}

// Use case input
export interface SubmitOrderInput {
  readonly orderId: OrderId;
  readonly submittedBy: string;
}

// Use case output
export interface SubmitOrderOutput {
  readonly orderId: OrderId;
  readonly totalAmount: Money;
  readonly itemCount: number;
  readonly submittedAt: Date;
}

// Use case implementation
export const createSubmitOrderUseCase = (deps: {
  orderRepository: OrderRepository;
  eventPublisher: EventPublisher;
  logger: Logger;
}) => {
  return async (input: SubmitOrderInput): AsyncResult<SubmitOrderOutput, DomainError> => {
    const { orderRepository, eventPublisher, logger } = deps;

    logger.info('Submitting order', { orderId: input.orderId });

    // 1. Load aggregate
    const loadResult = await orderRepository.findById(input.orderId);
    if (isFailure(loadResult)) {
      return loadResult;
    }

    const order = loadResult.value;
    if (order === null) {
      return failure(notFoundError('Order', input.orderId));
    }

    // 2. Execute command
    const command: SubmitOrder = { _tag: 'SubmitOrder' };
    const commandResult = handleOrderCommand(order, command);

    if (isFailure(commandResult)) {
      logger.error('Failed to submit order', commandResult.error, { orderId: input.orderId });
      return commandResult;
    }

    const events = commandResult.value;

    // 3. Apply events to get new state
    const newState = events.reduce(applyOrderEvent, order);

    // 4. Persist
    const saveResult = await orderRepository.save(newState, events);
    if (isFailure(saveResult)) {
      return saveResult;
    }

    // 5. Publish events
    const publishResult = await eventPublisher.publish(events);
    if (isFailure(publishResult)) {
      // Log but don't fail - events can be retried
      logger.error('Failed to publish events', publishResult.error, { orderId: input.orderId });
    }

    // 6. Build output
    const submittedEvent = events.find(e => e._tag === 'OrderSubmitted') as OrderSubmitted;

    logger.info('Order submitted successfully', {
      orderId: input.orderId,
      totalAmount: submittedEvent.totalAmount,
    });

    return success({
      orderId: input.orderId,
      totalAmount: submittedEvent.totalAmount,
      itemCount: submittedEvent.itemCount,
      submittedAt: submittedEvent.occurredAt,
    });
  };
};

// Type for the use case function
export type SubmitOrderUseCase = ReturnType<typeof createSubmitOrderUseCase>;
```

---

## 7. HTTP Controller (Express/Fastify)

```typescript
// infrastructure/http/order-controller.ts

import { Router, Request, Response } from 'express';

export const createOrderController = (deps: {
  submitOrder: SubmitOrderUseCase;
  // other use cases...
}) => {
  const router = Router();

  router.post('/:orderId/submit', async (req: Request, res: Response) => {
    const input: SubmitOrderInput = {
      orderId: req.params.orderId as OrderId,
      submittedBy: req.user?.id ?? 'anonymous',
    };

    const result = await deps.submitOrder(input);

    if (isFailure(result)) {
      const error = result.error;

      // Map domain errors to HTTP status codes
      switch (error._tag) {
        case 'ValidationError':
          return res.status(400).json({
            error: 'validation_error',
            field: error.field,
            message: error.message,
          });

        case 'NotFoundError':
          return res.status(404).json({
            error: 'not_found',
            entity: error.entity,
            id: error.id,
          });

        case 'BusinessRuleViolation':
          return res.status(422).json({
            error: 'business_rule_violation',
            rule: error.rule,
            message: error.message,
          });

        case 'AuthorizationError':
          return res.status(403).json({
            error: 'forbidden',
            message: error.reason ?? 'Access denied',
          });

        case 'ConflictError':
          return res.status(409).json({
            error: 'conflict',
            message: error.reason,
          });
      }
    }

    return res.status(200).json(result.value);
  });

  return router;
};
```

---

## 8. Testing (Property-Based + Unit)

```typescript
// tests/domain/order.test.ts

import { fc } from 'fast-check';

describe('Order Aggregate', () => {
  describe('AddItem command', () => {
    it('should reject negative quantities', () => {
      const order = createDraftOrder();
      const command: AddItem = {
        _tag: 'AddItem',
        productId: 'prod_123' as ProductId,
        quantity: -1,
        unitPrice: { amount: 10, currency: 'USD' },
      };

      const result = handleOrderCommand(order, command);

      expect(isFailure(result)).toBe(true);
      if (isFailure(result)) {
        expect(result.error._tag).toBe('ValidationError');
      }
    });

    it('should reject items with mismatched currency', () => {
      const order = createDraftOrder({ currency: 'USD' });
      const command: AddItem = {
        _tag: 'AddItem',
        productId: 'prod_123' as ProductId,
        quantity: 1,
        unitPrice: { amount: 10, currency: 'EUR' },
      };

      const result = handleOrderCommand(order, command);

      expect(isFailure(result)).toBe(true);
      if (isFailure(result)) {
        expect(result.error._tag).toBe('BusinessRuleViolation');
      }
    });

    // Property-based test
    it('should always produce valid order state', () => {
      fc.assert(
        fc.property(
          fc.integer({ min: 1, max: 100 }),
          fc.double({ min: 0.01, max: 1000, noNaN: true }),
          (quantity, price) => {
            const order = createDraftOrder();
            const command: AddItem = {
              _tag: 'AddItem',
              productId: 'prod_123' as ProductId,
              quantity,
              unitPrice: { amount: price, currency: 'USD' },
            };

            const result = handleOrderCommand(order, command);

            if (isSuccess(result)) {
              const newState = result.value.reduce(applyOrderEvent, order);
              expect(newState.items.length).toBe(order.items.length + 1);
              expect(newState.version).toBe(order.version + 1);
            }

            return true;
          }
        )
      );
    });
  });

  describe('SubmitOrder command', () => {
    it('should reject empty orders', () => {
      const order = createDraftOrder({ items: [] });
      const command: SubmitOrder = { _tag: 'SubmitOrder' };

      const result = handleOrderCommand(order, command);

      expect(isFailure(result)).toBe(true);
      if (isFailure(result)) {
        expect(result.error._tag).toBe('BusinessRuleViolation');
        expect((result.error as BusinessRuleViolation).rule).toBe('EmptyOrder');
      }
    });

    it('should calculate correct total', () => {
      const order = createDraftOrder({
        items: [
          { productId: 'p1' as ProductId, quantity: 2, unitPrice: { amount: 10, currency: 'USD' } },
          { productId: 'p2' as ProductId, quantity: 3, unitPrice: { amount: 5, currency: 'USD' } },
        ],
      });
      const command: SubmitOrder = { _tag: 'SubmitOrder' };

      const result = handleOrderCommand(order, command);

      expect(isSuccess(result)).toBe(true);
      if (isSuccess(result)) {
        const event = result.value[0] as OrderSubmitted;
        expect(event.totalAmount.amount).toBe(35); // (2*10) + (3*5)
      }
    });
  });
});

// Test helpers
const createDraftOrder = (overrides?: Partial<Order>): Order => ({
  id: 'order_123' as OrderId,
  customerId: 'cust_456' as CustomerId,
  status: 'draft',
  items: [],
  currency: 'USD',
  version: 1,
  createdAt: new Date(),
  updatedAt: new Date(),
  ...overrides,
});
```

---

## Directory Structure

```
src/
├── core/
│   ├── result.ts              # Result type utilities
│   └── types.ts               # Common type utilities
├── domain/
│   ├── errors.ts              # Domain error types
│   ├── events.ts              # Domain event types
│   ├── value-objects/
│   │   ├── email.ts
│   │   ├── money.ts
│   │   └── entity-id.ts
│   └── aggregates/
│       ├── order/
│       │   ├── order.ts       # Aggregate state
│       │   ├── commands.ts    # Command types
│       │   ├── events.ts      # Order-specific events
│       │   └── handlers.ts    # Command handlers
│       └── customer/
│           └── ...
├── application/
│   ├── ports/                 # Interface definitions
│   │   ├── repositories.ts
│   │   └── services.ts
│   └── use-cases/
│       ├── submit-order.ts
│       ├── cancel-order.ts
│       └── ...
├── infrastructure/
│   ├── persistence/
│   │   ├── mongodb/
│   │   │   └── order-repository.ts
│   │   └── postgres/
│   │       └── order-repository.ts
│   ├── messaging/
│   │   └── event-publisher.ts
│   └── http/
│       ├── server.ts
│       └── controllers/
│           └── order-controller.ts
└── main.ts                    # Composition root
```

---

## Key Benefits

1. **Type Safety**: The compiler catches most errors before runtime
2. **Exhaustive Checking**: `switch` on discriminated unions forces handling all cases
3. **Testability**: Pure functions for domain logic, dependency injection for infrastructure
4. **Evolvability**: Adding new events/commands is additive, not breaking
5. **LLM-Friendly**: Clear patterns that AI assistants can follow and extend
6. **No Exceptions**: Explicit error handling through Result types
7. **Immutability**: Prevents accidental mutations and race conditions

## Comparison to Java Sealed Classes

| TypeScript | Java |
|------------|------|
| `type DomainError = ValidationError \| NotFoundError` | `sealed interface DomainError permits ValidationError, NotFoundError` |
| `{ _tag: 'ValidationError', field, message }` | `record ValidationError(String field, String message) implements DomainError` |
| `switch (error._tag)` | `switch (error) { case ValidationError v -> ... }` |
| Runtime discrimination via `_tag` | Compile-time type discrimination |

Both approaches achieve the same goal: making illegal states unrepresentable and forcing exhaustive handling of all variants.
