# Message Groups & FIFO Ordering Guide

FlowCatalyst supports per-message-group FIFO ordering for scenarios requiring strict event sequencing. This guide explains message groups, dispatch modes, and ordering guarantees.

## Overview

Message groups provide:
- **Strict ordering** within a group
- **Parallel processing** across groups
- **Configurable error handling** per subscription

```
Message Group: order:123     Message Group: order:456
┌───────────────────────┐    ┌───────────────────────┐
│ Event 1 ──► Event 2   │    │ Event A ──► Event B   │
│    │           │      │    │    │           │      │
│    ▼           ▼      │    │    ▼           ▼      │
│ Job 1  ──►  Job 2     │    │ Job A  ──►  Job B     │
└───────────────────────┘    └───────────────────────┘
        │                            │
        └──── Process in parallel ───┘
```

## Message Group Format

### Event Message Group

Events specify their message group via the `messageGroup` field:

```json
{
  "type": "order.created",
  "subject": "order:12345",
  "messageGroup": "order:12345",
  "data": {...}
}
```

Common patterns:
- **By aggregate**: `order:12345`, `customer:789`
- **By tenant**: `client:acme`
- **By resource**: `inventory:sku-abc`

### Dispatch Job Message Group

For dispatch jobs, the group combines subscription and event:

```
{subscriptionCode}:{eventMessageGroup}
```

Example:
- Subscription: `order-notifications`
- Event group: `order:12345`
- Job group: `order-notifications:order:12345`

This ensures ordering within each subscription independently.

## Dispatch Modes

### IMMEDIATE (Default)

Process messages as fast as possible, no ordering guarantees.

```
Messages arrive: 1, 2, 3
Processing:      1, 3, 2 (any order)
```

**Use when**: Order doesn't matter, maximum throughput needed.

### NEXT_ON_ERROR

Maintain order within group, but skip failed messages:

```
Messages: 1, 2, 3
Process 1: Success
Process 2: FAIL (skip, continue)
Process 3: Success
```

**Use when**: Order matters but failures shouldn't block.

### BLOCK_ON_ERROR

Strict ordering - block entire group on failure:

```
Messages: 1, 2, 3
Process 1: Success
Process 2: FAIL
Process 3: BLOCKED (waiting for 2)
```

**Use when**: Strict ordering required, failures need resolution.

## Configuration

### Subscription Level

```json
{
  "code": "order-processor",
  "mode": "NEXT_ON_ERROR",
  "eventTypes": [
    {"eventTypeCode": "order.created"},
    {"eventTypeCode": "order.updated"}
  ]
}
```

### Dispatch Job Level

Jobs inherit mode from subscription but can override:

```json
{
  "kind": "TASK",
  "code": "critical-update",
  "messageGroup": "account:123",
  "mode": "BLOCK_ON_ERROR"
}
```

## Sequence Numbers

Within a message group, sequence numbers control order:

```json
{
  "messageGroup": "order:12345",
  "sequence": 1,  // Lower = process first
  "code": "order.created"
}

{
  "messageGroup": "order:12345",
  "sequence": 2,
  "code": "order.shipped"
}
```

Default sequence: 99

### Sequence Assignment

For events, sequence typically comes from the event or subscription:

```json
{
  "type": "order.created",
  "messageGroup": "order:12345",
  "extensions": {
    "sequence": 1
  }
}
```

## Implementation Details

### Queue Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Message Router                        │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────┐   Per-group tracking                │
│  │ Group: order:1│   ┌─────────────────────────┐       │
│  │ - Msg 1 (done)│   │ ConcurrentHashMap       │       │
│  │ - Msg 2 (proc)│   │ group → in-flight count │       │
│  │ - Msg 3 (wait)│   └─────────────────────────┘       │
│  └───────────────┘                                      │
│                                                         │
│  ┌───────────────┐   Virtual Thread Pool               │
│  │ Group: order:2│   ┌─────────────────────────┐       │
│  │ - Msg A (proc)│   │ Thread per message      │       │
│  │ - Msg B (wait)│   │ (concurrent across      │       │
│  └───────────────┘   │  groups)                │       │
│                      └─────────────────────────┘       │
└─────────────────────────────────────────────────────────┘
```

### Processing Rules

1. **Different groups**: Process concurrently
2. **Same group, IMMEDIATE**: Process concurrently
3. **Same group, NEXT_ON_ERROR**: Sequential, skip failures
4. **Same group, BLOCK_ON_ERROR**: Sequential, block on failure

### Error Handling

**NEXT_ON_ERROR**:
```
Job 2 fails → Status = ERROR
Job 3 → Processes immediately
Job 2 → Retries later (independent)
```

**BLOCK_ON_ERROR**:
```
Job 2 fails → Status = ERROR
Job 3 → Status = PENDING (blocked)
Job 2 retries...
Job 2 succeeds → Job 3 unblocked
```

## Best Practices

### When to Use Message Groups

| Scenario | Use Group? | Mode |
|----------|------------|------|
| Order lifecycle events | Yes | NEXT_ON_ERROR |
| Financial transactions | Yes | BLOCK_ON_ERROR |
| Notification broadcasts | No | IMMEDIATE |
| Audit logging | Optional | NEXT_ON_ERROR |

### Group Design

1. **Choose appropriate granularity**
   - Too broad: Reduces parallelism
   - Too narrow: Loses ordering benefits

2. **Use consistent group keys**
   - Same aggregate → Same group
   - `order:12345` not `order_12345`

3. **Consider failure impact**
   - BLOCK_ON_ERROR can cause backlogs
   - Plan for error resolution

### Performance Considerations

1. **Avoid hot groups** - High-volume single groups reduce throughput
2. **Balance group size** - More groups = more parallelism
3. **Monitor blocked jobs** - Alert on BLOCK_ON_ERROR backlogs

## Monitoring

### Metrics

| Metric | Description |
|--------|-------------|
| `dispatch.groups.active` | Active message groups |
| `dispatch.groups.blocked` | Blocked groups (BLOCK_ON_ERROR) |
| `dispatch.messages.in_group` | Messages per group (histogram) |

### Queries

```http
# Find blocked jobs
GET /api/dispatch-jobs?status=PENDING&mode=BLOCK_ON_ERROR

# Find jobs in specific group
GET /api/dispatch-jobs?messageGroup=order:12345
```

## Troubleshooting

### Jobs Stuck in PENDING

1. Check if blocking job is in ERROR state
2. Review error details in attempt history
3. Fix issue and retry blocking job
4. Or change mode to NEXT_ON_ERROR

### Out-of-Order Processing

1. Verify events have correct `messageGroup`
2. Check sequence numbers
3. Confirm subscription mode setting
4. Review router logs for routing decisions

## See Also

- [Dispatch Jobs Guide](dispatch-jobs.md) - Job delivery
- [Message Router](../architecture/message-router.md) - Router architecture
- [Subscription Entities](../entities/subscription-entities.md) - Entity reference
