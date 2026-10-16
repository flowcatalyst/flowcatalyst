# Changes

## Unreleased

Additive only; nothing removed, renamed or behaviour-changed (the only runtime difference is the
deprecation notice described below).

- New `platformConfig()` resource: `get`, `getRaw`, `set`, `delete`, `list`, `listRaw`
  (`/api/config/{app}/{section}/{property}`, `/api/platform-config/{app}`).
- New `emailDomainMappings()` resource: `getByDomain`, `getByDomainRaw`
  (`GET /api/email-domain-mappings/by-domain/{domain}`).
- `principals()->setClientAssociation($id, $clientId, ?$mode)`
  (`PUT /api/principals/{id}/client-association`, anchor-only).
- `clients()->getRaw($id)` and `getByIdentifierRaw($identifier)` return the platform's
  decoded body unchanged (the `Client` DTO defaults a missing `status` to `ACTIVE`).
- `roles()->sync()` failures from the atomic platform sync ("... failed at index N") now
  append the failing role name and the platform's error code/details to the message and
  add `failedIndex`/`failedRole` to the exception context. The original message is kept
  as the prefix, the code is unchanged and the original exception is chained as `previous`.
  Subclasses (e.g. `ValidationException`) are not touched.
- Tests pin `EventTypeDefinition::toArray()['code']` and `AddSchemaVersionRequest` always
  sending `version` (both already correct since 0.10.0; apps on older versions should upgrade
  rather than keep their workarounds).

### `admin()` facade and deprecation notices

- New `FlowCatalystClient::admin(): Admin` (also `FlowCatalyst::admin()`), exposing `principals()`,
  `clients()`, `applications()` and `router()`. They are the same resource classes as the old
  accessors and behave identically; the facade is just the quiet path.
- The old accessors keep working. These methods now raise one `E_USER_DEPRECATED` per method per
  process (suppressible via `error_reporting()`/`@`/handler; never raised through `admin()`), and
  carry `@deprecated` docblocks:
  - `principals()`: `list`, `get`, `findByEmail`, `createUser`, `resetPassword`, `update`, `activate`,
    `deactivate`, `getRoles`, `addRole`, `removeRole`, `setRoles`, `getClientAccessGrants`,
    `grantClientAccess`, `revokeClientAccess`.
  - `clients()`: `search`, `create`, `update`, `activate`, `suspend`, `deactivate`, `addNote`,
    `getApplications`, `updateApplications`, `enableApplication`, `disableApplication`.
  - `applications()`: `list`, `create`, `update`, `delete`, `activate`, `deactivate`,
    `provisionServiceAccount`, `getServiceAccount`, `listClients`, `updateClientConfig`,
    `enableForClient`, `disableForClient`.
  - `router()`: `inPipeline`, `inPipelineBatch`.
- Not deprecated: all `sync*` methods (`principals()->sync/syncUsers`, `roles()->sync`, ...),
  `clients()->list/get/getByIdentifier/getRaw/getByIdentifierRaw`,
  `applications()->get/getByCode/listRoles`, the new `principals()->setClientAssociation`,
  platform config, email-domain mappings, outbox, dispatch-job creation, auth, `request()` and the
  constructor options. Nothing is removed.
- Test doubles: a Mockery/subclass double that stubs `principals()` etc. does not intercept
  `admin()`; stub `admin()` (the `Admin` class is not final) when moving a call site.

Known issue, not fixed here: `Clients::update()` PUTs and then parses a body with
`Client::fromArray()`, but the platform answers `204 No Content`, so the decoded body is empty and
the call fails (missing `id`). Use `get($id)` after an update until it is fixed.

### Migrating to admin()

Pure search/replace on the accessor chain; arguments are unchanged. Paths are relative to
`/Users/andrewgraaff/Developer/inhance/InhanceMono`. Calls that stay as they are are listed so nobody
touches them by mistake. A test double that stubs the accessor (`shouldReceive('principals')` /
`applications`) must instead stub `admin()` returning an `Admin` whose `principals()` etc. return the
fake.

**apps/hr** (pinned `^0.8`, locked 0.8.19: `admin()` needs a constraint change and a 0.8 to 0.10 review first)

| File | Replace | With |
|---|---|---|
| `packages/Foundation/Domain/Services/PrincipalDirectory.php:151` | `$this->client->principals()->list(active: true)` | `$this->client->admin()->principals()->list(active: true)` |
| `packages/Register/Commands/ImportRolesRegisterCommand.php:275` | `resolve(FlowCatalystClient::class)->principals()->list(type: 'USER', active: true)` | `resolve(FlowCatalystClient::class)->admin()->principals()->list(type: 'USER', active: true)` |
| `packages/Foundation/Domain/Services/FeedbackNotifyRecipients.php:83-86` | `$applications = $this->client->applications(); $applications->getByCode(...)` then `$applications->listClients(...)` | keep `getByCode` on `$this->client->applications()`; change only `->listClients($application->id)` to `$this->client->admin()->applications()->listClients($application->id)` |
| `app/Console/Commands/SyncTestPrincipalsCommand.php:57` | `principals()->sync(` | unchanged (sync stays) |
| tests: `tests/Feature/Foundation/PrincipalDirectoryTest.php`, `FeedbackNotifyRecipientsTest.php` | `$client->shouldReceive('principals')` / `('applications')` for the moved calls | `shouldReceive('admin')` returning a mock `Admin` |

**apps/rfp**

| File | Replace | With |
|---|---|---|
| `packages/Tender/Infrastructure/Directory/PlatformPrincipalGateway.php:51` | `$this->client->principals()->findByEmail(` | `$this->client->admin()->principals()->findByEmail(` |
| same, `:60` | `$this->client->principals()->get(` | `$this->client->admin()->principals()->get(` |
| same, `:106` | `$this->client->principals()->createUser(` | `$this->client->admin()->principals()->createUser(` |
| same, `:128`, `:141` | `$this->client->principals()->addRole(` | `$this->client->admin()->principals()->addRole(` |
| same, `:135` | `$this->client->principals()->removeRole(` | `$this->client->admin()->principals()->removeRole(` |
| `packages/Tender/Infrastructure/Directory/PlatformPrincipalDirectory.php:223` | `$this->client->principals()->list(active: true)` | `$this->client->admin()->principals()->list(active: true)` |
| `app/Console/Commands/SyncTestPrincipalsCommand.php:57` | `principals()->sync(` | unchanged |
| `tests/Architecture/PrincipalWritersTest.php` | pattern that bans `->principals()` chains / `$p = $this->client->principals()` | extend to allow the single gateway to use `->admin()->principals()` and ban it elsewhere |
| `tests/Doubles/ScriptedFlowCatalystClient.php` | overrides at the `request()` level | no change expected (resources route through `request()`); verify |

**apps/integral**

| File | Replace | With |
|---|---|---|
| `app/Console/Commands/FlowCatalyst/CreateFlowCatalystUserCommand.php:103` | `$client->principals()->createUser(` | `$client->admin()->principals()->createUser(` |
| `app/Console/Commands/FlowCatalyst/SyncUsersToFlowCatalystCommand.php:381` | `$client->principals()->findByEmail(` | `$client->admin()->principals()->findByEmail(` |
| same, `:395` | `$client->principals()->update(` | `$client->admin()->principals()->update(` |
| same, `:440` | `$client->principals()->createUser(` | `$client->admin()->principals()->createUser(` |
| same, `:483` | `$client->principals()->getRoles(` | `$client->admin()->principals()->getRoles(` |
| same, `:490` | `$client->principals()->addRole(` | `$client->admin()->principals()->addRole(` |
| same, `:554` | `$client->principals()->grantClientAccess(` | `$client->admin()->principals()->grantClientAccess(` |
| same (raw `request('PUT', '/api/principals/{id}/client-association'...)`) | `$client->request(...)` | `$client->admin()->principals()->setClientAssociation($id, $clientId, $mode)` |
| same, `:163` | `$client->clients()->list()` | unchanged (not deprecated) |
| same, `:599` | `$client->principals()->syncUsers(` | unchanged |

**packages_root** (shared by integral)

| File | Replace | With |
|---|---|---|
| `packages/integral-service-v2/src/Admin/Http/Controllers/SetPasswordController.php:92` | `$this->flowCatalystClient->principals()->findByEmail(` | `...->admin()->principals()->findByEmail(` |
| same, `:98` | `...->principals()->resetPassword(` | `...->admin()->principals()->resetPassword(` |
| `.../Admin/UseCases/ResetUserPasswordUseCase.php:62`, `:68` | `->principals()->findByEmail(` / `->resetPassword(` | `->admin()->principals()->...` |
| `.../Admin/UseCases/UpdateUserUseCase.php:78`, `:84` | `->principals()->findByEmail(` / `->resetPassword(` | `->admin()->principals()->...` |
| `.../Admin/UseCases/CreateUserUseCase.php:122` | `->principals()->createUser(` | `->admin()->principals()->createUser(` |
| `.../Admin/Jobs/RunTenantMigrationsJob.php:137` | `$fc->clients()->create(` | `$fc->admin()->clients()->create(` |
| `.../FlowCatalyst/Commands/SyncFlowCatalystClientsCommand.php:95` | `$this->client->clients()->create(` | `$this->client->admin()->clients()->create(` |
| same, `:59` | `$this->client->clients()->list()` | unchanged (not deprecated) |
| `.../FlowCatalyst/Commands/SyncFlowCatalystDefinitionsCommand.php:600` | `$this->client->applications()->create(` | `$this->client->admin()->applications()->create(` |
| same, `:593` | `applications()->getByCode(` | unchanged |
| `.../ProcessRecord/ProcessRecordInProgressResetterJob.php:90` | `$client->router()->areInPipeline($messageIds)` | `$client->admin()->router()->inPipelineBatch($messageIds)` (see note) |
| `packages/integral-service-v1/src/Legacy/Services/FlowCatalystSyncService.php:100` | `$this->client->applications()->create([...array...])` | `$this->client->admin()->applications()->create(new CreateApplicationRequest(code:, name:, description:))` (see note) |
| same, `:95` | `applications()->getByCode(` | unchanged |

Notes found while checking call sites:

- `areInPipeline` does not exist in the SDK (and never did in this repo's history): the SDK's
  `Router` has `inPipeline(string)` and `inPipelineBatch(array)` (returns `messageId => bool`, which is
  what the Integral caller expects). The current v2 call would throw `Error: undefined method`; today
  the call is inside a `catch (Throwable)` that returns `[]`, and the surrounding code is partly
  commented out. The replacement above is the correct call, and the job should be re-enabled
  deliberately, not as part of the rename.
- The v1 `applications()->create([...])` passes an array to a method typed
  `CreateApplicationRequest`; it only works on an older SDK line. Moving to `admin()` also needs the
  request object.
- Every method the apps use has an `admin()` equivalent; none of the used methods is missing.
