# API Response Types Changes - Frontend Migration Guide

## Summary

Backend API responses have been refactored to use properly typed DTOs instead of untyped `Map.of()` or `unknown` responses. This enables proper TypeScript type generation from OpenAPI.

## New Common Response DTOs

All response types are now defined in `/core/flowcatalyst-platform/src/main/java/tech/flowcatalyst/platform/common/api/`:

### ApiResponses.java (Common Types)
- `MessageResponse` - Simple message response
- `DeleteResponse` - DELETE operations now return `{ id, resourceType, message }` with HTTP 200 (not 204)
- `StatusChangeResponse` - For status change operations
- `ErrorResponse` - 400 errors with `{ code, message, details }`
- `ValidationErrorResponse` - Field-level validation errors
- `NotFoundResponse` - 404 errors with `{ code, message, resourceType, resourceId }`
- `ConflictResponse` - 409 errors
- `UnauthorizedResponse` - 401 errors
- `ForbiddenResponse` - 403 errors with optional `requiredPermission`

### ApplicationResponses.java
- `ApplicationListItem` - Summary for list views
- `ApplicationListResponse` - `{ applications, total }`
- `ApplicationResponse` - Full details with optional `serviceAccount` and `warning`
- `ApplicationStatusResponse` - Status changes with `{ id, status, message }`
- `ClientConfigResponse` - Application-client configuration
- `ClientConfigListResponse` - List of client configs
- `ClientApplicationStatusResponse` - Enable/disable for client

### ClientResponses.java
- `ClientDto` - Client details
- `ClientListResponse` - `{ clients, total }`
- `ClientStatusResponse` - Status changes with `{ id, status, message }`
- `ClientApplicationDto` - Application with enabled status
- `ClientApplicationsResponse` - Applications for a client
- `ClientApplicationStatusResponse` - Enable/disable application for client
- `NoteAddedResponse` - Audit note creation response
- `ApplicationsUpdatedResponse` - Bulk update response

### AuditLogResponses.java
- `AuditLogDto` - Summary
- `AuditLogDetailDto` - Full details with operationJson
- `AuditLogListResponse` - Paginated list
- `EntityAuditLogsResponse` - Logs for specific entity
- `EntityTypesResponse` - Distinct entity types
- `OperationsResponse` - Distinct operations

### CorsResponses.java
- `CorsOriginDto` - CORS origin entry
- `CorsOriginListResponse` - List of origins
- `AllowedOriginsResponse` - Set of allowed origins
- `CorsOriginDeletedResponse` - DELETE response

## Key Changes for Frontend

### 1. DELETE Operations Return 200 with Body
All DELETE operations now return HTTP 200 with a response body containing the deleted resource ID:

```typescript
// Before: HTTP 204 No Content
await api.deleteApplication(id); // void

// After: HTTP 200 with body
const result = await api.deleteApplication(id);
// result = { id: "app_123", resourceType: "application", message: "Application deleted successfully" }
```

### 2. Error Responses are Now Typed
All 400/401/403/404 responses have proper schemas:

```typescript
// TypeScript types will be generated:
interface ErrorResponse {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

interface NotFoundResponse {
  code: string;  // "NOT_FOUND"
  message: string;
  resourceType: string;
  resourceId: string | null;
}

interface ForbiddenResponse {
  code: string;  // "FORBIDDEN"
  message: string;
  requiredPermission?: string;
}
```

### 3. Status Changes Have Consistent Format
All status change endpoints return the same structure:

```typescript
interface StatusResponse {
  id: string;
  status: string;
  message: string;
}

// Applies to:
// - activateApplication, deactivateApplication
// - activateClient, suspendClient, deactivateClient
// - etc.
```

### 4. List Responses Include Total
All list endpoints now return consistent structure:

```typescript
interface ListResponse<T> {
  items: T[];  // or specific name like 'applications', 'clients'
  total: number;
}
```

## Files Updated

### Fully Updated (using ApiResponses)
- `ApplicationAdminResource.java`
- `ClientAdminResource.java`
- `AuditLogAdminResource.java`
- `CorsAdminResource.java`
- `RoleAdminResource.java`

### Partially Updated (still have local ErrorResponse)
These files need updating to use `ApiResponses.ErrorResponse`:
- `PrincipalAdminResource.java`
- `OAuthClientAdminResource.java`
- `IdentityProviderAdminResource.java`
- `EmailDomainMappingAdminResource.java`
- `ConfigAdminResource.java`
- `ConfigAccessAdminResource.java`

## Frontend Migration Steps

1. **Regenerate OpenAPI spec** after backend changes
2. **Regenerate TypeScript SDK** using openapi-ts
3. **Update error handling** to use typed error responses
4. **Update DELETE callbacks** to expect response body instead of void
5. **Update status change handlers** to use new response format
6. **Remove any `as unknown` or type assertions** that were workarounds

## SDK Changes Made

### TypeScript SDK
- Fixed `errAsync` vs `err` usage in `client.ts`
- Added `StatusResponse` interface
- Added client application methods

### Laravel SDK
- Added `TokenProviderInterface` and `UserTokenProvider`
- Updated `FlowCatalystClient` to support user tokens
- Added client application methods
