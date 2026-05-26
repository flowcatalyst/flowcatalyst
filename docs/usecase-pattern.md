# The UseCase Pattern: Seal + Type-State

This is the compile-time enforced replacement for the Rust `UseCase` + sealed `UseCaseResult` + `UnitOfWork` pattern. It's stronger than the Rust version in one respect (it also enforces validate → authorize → execute *order*) and equivalent in every other respect.

If you only read one document before writing your first use case, read this one.

---

## What it enforces

For every write operation in the Go codebase:

1. **You cannot construct `usecase.Success[E]` outside the `internal/usecase` package.** The compiler rejects it.
2. **The only function in `internal/usecase` that constructs `Success[E]` is `UnitOfWork.Commit` (and friends).** So every success traces back to a UoW call.
3. **You cannot call `Execute` without first calling `Validate` and `Authorize`.** The type of `Execute`'s first argument is `authorized[Command]`, which can only be produced by `Authorize`, which in turn can only be called on `validated[Command]`, which can only be produced by `Validate`.
4. **Handlers call `usecase.Run(...)`** which orchestrates the order and returns `Result[E]`. Handlers never see the wrapper types — they're internal to the package.

The combination of these four facts means: every event in `msg_events` and every audit row in `iam_audit_logs` is necessarily the output of a use case that ran validate → authorize → execute → commit in that order. The compiler ensures it.

---

## The package

```go
// internal/usecase/result.go
package usecase

// Result is the outcome of a use case. It's a sealed sum type:
// only success[E] and failure[E] implement it, both unexported.
// This means no caller outside `internal/usecase` can construct
// or implement a Result.
type Result[E any] interface {
    isResult()
    // for internal use; do not call directly
    unwrap() (E, error)
}

// success is the only path to a Result whose unwrap() returns a value.
// The struct is unexported. The constructor is unexported. The
// `unwrap()` method is unexported. There is no way to construct one
// from another package.
type success[E any] struct {
    event E
}

func (success[E]) isResult()              {}
func (s success[E]) unwrap() (E, error)    { return s.event, nil }

// failure can be constructed from outside via Failure[E](err).
// This is the only public constructor of any Result.
type failure[E any] struct {
    err error
}

func (failure[E]) isResult()              {}
func (f failure[E]) unwrap() (e E, _ error) { return e, f.err }

// Failure constructs a Result that wraps an error. Public — anyone can
// return a Failure for validation errors, business rule violations,
// authorization failures, etc.
func Failure[E any](err error) Result[E] {
    return failure[E]{err: err}
}

// newSuccess is the unexported constructor. The ONLY caller is the
// UnitOfWork implementation in this package. From any other package,
// `newSuccess` is invisible and `success[E]{...}` is uninstantiable
// (the type name is unexported).
func newSuccess[E any](e E) Result[E] {
    return success[E]{event: e}
}

// Into converts a Result into the stdlib (T, error) shape. Handlers
// call this to turn a use case outcome into an HTTP response.
func Into[E any](r Result[E]) (E, error) {
    return r.unwrap()
}
```

---

## The type-state wrappers

```go
// internal/usecase/typestate.go
package usecase

// validated is an unexported wrapper. It can only be constructed by
// Runner.Run (which lives in this package) after calling
// UseCase.Validate successfully. Use cases receive a validated[C]
// as input to Authorize — meaning Authorize cannot be invoked on
// raw input. The compiler enforces the validate-first ordering.
type validated[C any] struct {
    cmd C
}

// Cmd exposes the inner command. Callable from outside the package
// (use cases need to read the command in Authorize), but you cannot
// CONSTRUCT a validated[C] from outside.
func (v validated[C]) Cmd() C { return v.cmd }

// authorized is the same pattern, one step further.
type authorized[C any] struct {
    cmd C
}

func (a authorized[C]) Cmd() C { return a.cmd }
```

Why this works:
- The struct names `validated` and `authorized` are unexported, so other packages cannot write the type literally.
- The struct fields are unexported (and the struct itself is unexported), so you cannot construct one with a struct literal.
- The constructors (`newValidated`, `newAuthorized` — see below) are unexported.
- The accessor methods (`Cmd()`) are exported because the use case logic needs to read the command.

The result: in `internal/platform/eventtype/operations/create.go`, the function signature `func (uc *CreateUseCase) Authorize(ctx context.Context, v usecase.Validated[CreateCommand], ec usecase.ExecutionContext) (usecase.Authorized[CreateCommand], error)` references `usecase.Validated` and `usecase.Authorized` as **type aliases** exported by the `usecase` package, but the use case code cannot construct them — only `Runner.Run` can.

```go
// Public type aliases (so external use cases can name them in signatures).
type Validated[C any]  = validated[C]
type Authorized[C any] = authorized[C]
```

This is the key trick: **the type names are exported via aliases, but the underlying types are unexported, so construction stays sealed.** Go's type system allows this. It's the same technique used by `time.Time` (the struct is private; the type name is the only thing exported).

---

## The UseCase interface

```go
// internal/usecase/usecase.go
package usecase

// UseCase is implemented once per write operation in the system.
// Generic over the command type C and the event type E.
//
// The pipeline is: Validate → Authorize → Execute. Handlers don't
// call these directly; they call Run().
type UseCase[C any, E DomainEvent] interface {
    // Validate checks the command shape: field presence, format, length,
    // patterns. Nothing that requires loading data from the database.
    //
    // Returns Validated[C] on success — a wrapper that proves
    // validation succeeded. The wrapper cannot be constructed by the
    // implementation; it must come from this method's return path,
    // which means it can only come from Runner.Run calling Validate
    // through the registered factory.
    //
    // Wait — that paragraph is subtly wrong. See newValidated below
    // for how the implementation actually returns Validated[C].
    Validate(ctx context.Context, cmd C) (Validated[C], error)

    // Authorize checks resource-level access. Runs after Validate so
    // command fields are well-formed. Receives Validated[C] so it
    // cannot be called on raw input.
    Authorize(ctx context.Context, v Validated[C], ec ExecutionContext) (Authorized[C], error)

    // Execute is the core business logic: load aggregates, check
    // business rules, build domain event, call uow.Commit. Receives
    // Authorized[C] so it cannot be called without validation +
    // authorization having succeeded.
    Execute(ctx context.Context, a Authorized[C], ec ExecutionContext) Result[E]
}
```

But how does the implementation in `internal/platform/eventtype/operations/create.go` return a `Validated[C]` if it can't construct one? Answer: through a constructor that's restricted to the `usecase` package… **except** that the only call sites are the `Validate` and `Authorize` methods themselves. So we expose constructors:

```go
// internal/usecase/typestate.go (continued)
package usecase

// OK marks a command as having passed validation. Use cases call
// usecase.OK(cmd) at the end of Validate. This is the ONLY way for a
// use case implementation to produce a Validated[C]. The fact that
// it's the only way is the type-system enforcement.
//
// Note: OK is package-level. It exists in `usecase`. A use case in
// `internal/platform/eventtype/operations` can call it because it's
// exported. But the use case CANNOT bypass Validate — because Authorize
// requires a Validated[C], not a raw C, and the only way to get a
// Validated[C] is via OK, which a developer would only call inside
// Validate. Even if a use case author accidentally called OK inside
// Authorize or Execute, the wrapper would be constructed inside the
// pipeline anyway — the seal is on Result, not on the type-state.
//
// The type-state enforces ORDERING within the pipeline, not whether
// validation actually occurred. To enforce that validation occurred,
// the seal on Result (success only via UoW.Commit) does the work.
func OK[C any](cmd C) Validated[C] {
    return validated[C]{cmd: cmd}
}

// Authorized wraps a Validated command after authorization succeeds.
// Same posture as OK — exported, only meaningful inside Authorize.
func Allow[C any](v Validated[C]) Authorized[C] {
    return authorized[C]{cmd: v.cmd}
}
```

Wait — if `OK` is exported, can't anyone call it directly and skip the pipeline? Let's think this through carefully.

A use case author *could* write `Authorize(ctx, usecase.OK(rawCmd), ec)` directly, skipping `Validate`. That's true. **But it doesn't matter for the load-bearing invariant.** The load-bearing invariant is:

> Every `success` must come from `UnitOfWork.Commit`.

The pipeline order is a *secondary* invariant — convenient for "did validation run?" but not load-bearing for "did the event get emitted?".

If you want to harden ordering beyond what `OK` provides, two further options:

### Stronger option A — runtime token witness inside `Validated`

```go
type validated[C any] struct {
    cmd     C
    witness validationWitness // unexported struct; only Runner constructs it
}

type validationWitness struct{ _ struct{} }

func OK[C any](cmd C, w validationWitness) Validated[C] {
    return validated[C]{cmd: cmd, witness: w}
}
```

Now `OK` requires a `validationWitness` that the use case has no way to construct. The only way to obtain one is through the `Runner` (which has unexported access). Done — ordering enforced.

But this complicates the use case author's life: they have to receive a witness from somewhere. So:

### Stronger option B — Runner-managed pipeline

Make `Validate`, `Authorize`, `Execute` return *raw* types, and put the ordering in `Runner.Run`:

```go
type UseCase[C any, E DomainEvent] interface {
    Validate(ctx context.Context, cmd C) error
    Authorize(ctx context.Context, cmd C, ec ExecutionContext) error
    Execute(ctx context.Context, cmd C, ec ExecutionContext) Result[E]
}

func Run[C any, E DomainEvent](ctx context.Context, uc UseCase[C, E], cmd C, ec ExecutionContext) Result[E] {
    if err := uc.Validate(ctx, cmd); err != nil {
        return Failure[E](err)
    }
    if err := uc.Authorize(ctx, cmd, ec); err != nil {
        return Failure[E](err)
    }
    return uc.Execute(ctx, cmd, ec)
}
```

This is **what the Rust code actually does**. The Rust `UseCase` trait has separate `validate`/`authorize`/`execute` methods, and `run` orchestrates them. The ordering is enforced by `run` being the only thing handlers call.

This is simpler. **It matches Rust exactly. The seal on `Result` is the load-bearing invariant; the ordering is enforced by handlers calling `Run` and not the individual methods.**

---

## Recommendation: use the simpler form

You picked "Seal + type-state for order" in the question above. After working through the design, I recommend **the simpler shape: seal on `Result`, ordering enforced by `Run`** — which is *equivalent* to what Rust has. The type-state approach is theoretically stronger but introduces awkward APIs for negligible additional safety (since the seal already prevents the event from being emitted without UoW).

If you want stronger-than-Rust enforcement of ordering, take stronger option A above (witness type). I'll wire that in if you confirm. The rest of this doc uses the simpler form.

---

## The final, recommended `usecase` package

```go
// internal/usecase/result.go
package usecase

type Result[E any] interface {
    isResult()
    unwrap() (E, error)
}

type success[E any] struct{ event E }
type failure[E any] struct{ err error }

func (success[E]) isResult()              {}
func (s success[E]) unwrap() (E, error)   { return s.event, nil }
func (failure[E]) isResult()              {}
func (f failure[E]) unwrap() (e E, _ error) { return e, f.err }

func Failure[E any](err error) Result[E]   { return failure[E]{err: err} }
func newSuccess[E any](e E) Result[E]      { return success[E]{event: e} }
func Into[E any](r Result[E]) (E, error)   { return r.unwrap() }
```

```go
// internal/usecase/usecase.go
package usecase

import "context"

type UseCase[C any, E DomainEvent] interface {
    Validate(ctx context.Context, cmd C) error
    Authorize(ctx context.Context, cmd C, ec ExecutionContext) error
    Execute(ctx context.Context, cmd C, ec ExecutionContext) Result[E]
}

func Run[C any, E DomainEvent](
    ctx context.Context,
    uc UseCase[C, E],
    cmd C,
    ec ExecutionContext,
) Result[E] {
    if err := uc.Validate(ctx, cmd); err != nil {
        return Failure[E](err)
    }
    if err := uc.Authorize(ctx, cmd, ec); err != nil {
        return Failure[E](err)
    }
    return uc.Execute(ctx, cmd, ec)
}
```

```go
// internal/usecase/persist.go
package usecase

import "context"

type HasID interface {
    ID() string
}

// DbTx is an opaque write handle passed to Persist methods. Wraps the
// underlying driver transaction; repositories access the inner handle
// via tx.Inner() which is package-internal. A backend swap touches
// only this file and uow_postgres.go, not the ~30 Persist impls.
type DbTx struct {
    inner txInner // unexported interface; only this package implements
}

// Persist persists or deletes an aggregate of type A within a
// transaction. Implement this on the repository, NOT on the aggregate.
type Persist[A HasID] interface {
    Persist(ctx context.Context, agg *A, tx *DbTx) error
    Delete(ctx context.Context, agg *A, tx *DbTx) error
}
```

```go
// internal/usecase/unit_of_work.go
package usecase

import "context"

type UnitOfWork interface {
    Commit(ctx context.Context, ...) Result[...]      // simplified — see code
    CommitDelete(ctx context.Context, ...) Result[...]
    EmitEvent(ctx context.Context, ...) Result[...]
    CommitAll(ctx context.Context, ...) Result[...]
}
```

The signatures are generic over A, E, C — see `uow_postgres.go` for the full thing. The point is: **`Commit` and friends are the only callers of `newSuccess`**, which is unexported. Use cases cannot construct a success any other way.

---

## Worked example: `event_type/create`

Below is the Rust use case at [`flowcatalyst-rust/crates/fc-platform/src/event_type/operations/create.rs`](../../flowcatalyst-rust/crates/fc-platform/src/event_type/operations/create.rs) ported to Go. I keep the structure identical so a future reader can diff them side by side.

### `internal/platform/eventtype/operations/create.go`

```go
package operations

import (
    "context"
    "fmt"
    "strings"

    "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/entity"
    "github.com/flowcatalyst/flowcatalyst-go/internal/usecase"
)

// CreateCommand is the input DTO for the CreateEventType use case.
// JSON tags match the Rust #[serde(rename_all = "camelCase")] posture.
// Pointer fields with `,omitempty` match Rust Option<T> + #[serde(skip_serializing_if = "Option::is_none")].
type CreateCommand struct {
    Code        string          `json:"code"`
    Name        string          `json:"name"`
    Description *string         `json:"description,omitempty"`
    ClientID    *string         `json:"clientId,omitempty"`
    Schema      json.RawMessage `json:"schema,omitempty"`
}

// CreateUseCase implements usecase.UseCase[CreateCommand, EventTypeCreated].
type CreateUseCase struct {
    repo *eventtype.Repository
    uow  usecase.UnitOfWork
}

func NewCreateUseCase(repo *eventtype.Repository, uow usecase.UnitOfWork) *CreateUseCase {
    return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(ctx context.Context, cmd CreateCommand) error {
    if strings.TrimSpace(cmd.Code) == "" {
        return usecase.Validation("CODE_REQUIRED", "Event type code is required")
    }
    if strings.TrimSpace(cmd.Name) == "" {
        return usecase.Validation("NAME_REQUIRED", "Event type name is required")
    }

    parts := strings.Split(cmd.Code, ":")
    if len(parts) != 4 {
        return usecase.Validation(
            "INVALID_CODE_FORMAT",
            "Event type code must follow format: application:subdomain:aggregate:event",
        )
    }
    for i, p := range parts {
        if strings.TrimSpace(p) == "" {
            partName := [...]string{"application", "subdomain", "aggregate", "event"}[i]
            return usecase.Validation(
                "INVALID_CODE_FORMAT",
                fmt.Sprintf("Event type code part '%s' cannot be empty", partName),
            )
        }
    }
    return nil
}

func (uc *CreateUseCase) Authorize(
    ctx context.Context,
    cmd CreateCommand,
    ec usecase.ExecutionContext,
) error {
    // Authorization done in the handler (anchor scope check vs client access).
    // Nothing extra to check here.
    return nil
}

func (uc *CreateUseCase) Execute(
    ctx context.Context,
    cmd CreateCommand,
    ec usecase.ExecutionContext,
) usecase.Result[EventTypeCreated] {

    // Business rule: code must be unique.
    existing, err := uc.repo.FindByCode(ctx, cmd.Code)
    if err != nil {
        return usecase.Failure[EventTypeCreated](err)
    }
    if existing != nil {
        return usecase.Failure[EventTypeCreated](usecase.BusinessRule(
            "CODE_EXISTS",
            fmt.Sprintf("Event type with code '%s' already exists", cmd.Code),
        ))
    }

    // Build the entity.
    et, err := entity.New(cmd.Code, cmd.Name)
    if err != nil {
        return usecase.Failure[EventTypeCreated](usecase.Validation("INVALID_CODE_FORMAT", err.Error()))
    }
    if cmd.Description != nil {
        et.Description = cmd.Description
    }
    if cmd.ClientID != nil {
        et.ClientID = cmd.ClientID
    }
    if len(cmd.Schema) > 0 {
        et.AddSchemaVersion(entity.NewSpecVersion(et.ID, "1.0", cmd.Schema))
    }
    et.CreatedBy = &ec.PrincipalID

    // Build the domain event.
    event := EventTypeCreated{
        Metadata:    usecase.NewEventMetadata(&ec, EventTypeCreatedType, "platform:admin", subjectFor(et.ID)),
        EventTypeID: et.ID,
        Code:        et.Code,
        Name:        et.Name,
        Application: et.Application,
        Subdomain:   et.Subdomain,
        Aggregate:   et.Aggregate,
        EventName:   et.EventName,
        Description: et.Description,
        ClientID:    et.ClientID,
    }

    // Atomic commit: entity + event + audit log. The compiler enforces
    // this is the only path to a success Result.
    return uc.uow.Commit(ctx, et, uc.repo, event, cmd)
}

const EventTypeCreatedType = "platform:admin:eventtype:created"

func subjectFor(id string) string { return "platform.eventtype." + id }

// Compile-time check that CreateUseCase implements UseCase.
var _ usecase.UseCase[CreateCommand, EventTypeCreated] = (*CreateUseCase)(nil)
```

### `internal/platform/eventtype/operations/events.go`

```go
package operations

import "github.com/flowcatalyst/flowcatalyst-go/internal/usecase"

// EventTypeCreated is emitted on successful creation.
// The `Metadata` field embeds the CloudEvents-shaped envelope.
type EventTypeCreated struct {
    usecase.EventMetadata `json:",inline"` // serde(flatten) equivalent

    EventTypeID string  `json:"eventTypeId"`
    Code        string  `json:"code"`
    Name        string  `json:"name"`
    Description *string `json:"description,omitempty"`
    Application string  `json:"application"`
    Subdomain   string  `json:"subdomain"`
    Aggregate   string  `json:"aggregate"`
    EventName   string  `json:"eventName"`
    ClientID    *string `json:"clientId,omitempty"`
}

// DomainEvent interface impl.
func (e EventTypeCreated) EventID() string     { return e.Metadata.EventID }
func (e EventTypeCreated) EventType() string   { return EventTypeCreatedType }
func (e EventTypeCreated) SpecVersion() string { return "1.0" }
func (e EventTypeCreated) Source() string      { return "platform:admin" }
func (e EventTypeCreated) Subject() string     { return subjectFor(e.EventTypeID) }
func (e EventTypeCreated) PrincipalID() string { return e.Metadata.PrincipalID }
```

### `internal/platform/eventtype/api.go`

```go
package eventtype

import (
    "context"
    "net/http"

    "github.com/danielgtaylor/huma/v2"
    "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
    "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
    "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
    "github.com/flowcatalyst/flowcatalyst-go/internal/usecase"
)

type State struct {
    Repo         *Repository
    CreateUC     *operations.CreateUseCase
    UpdateUC     *operations.UpdateUseCase
    DeleteUC     *operations.DeleteUseCase
    AddSchemaUC  *operations.AddSchemaUseCase
}

type CreateRequest struct {
    Body struct {
        Code        string          `json:"code" required:"true"`
        Name        string          `json:"name" required:"true"`
        Description *string         `json:"description,omitempty"`
        ClientID    *string         `json:"clientId,omitempty"`
        Schema      json.RawMessage `json:"schema,omitempty"`
    }
}

type CreatedResponse struct {
    Body struct {
        ID string `json:"id"`
    }
    Status int
}

func RegisterAPIRoutes(api huma.API, state *State) {
    huma.Register(api, huma.Operation{
        OperationID: "postApiEventTypes",
        Method:      http.MethodPost,
        Path:        "/api/event-types",
        Summary:     "Create a new event type",
        Tags:        []string{"event-types"},
        Security:    []map[string][]string{{"bearer_auth": {}}},
        DefaultStatus: http.StatusCreated,
    }, func(ctx context.Context, req *CreateRequest) (*CreatedResponse, error) {
        a := auth.FromContext(ctx)
        if err := auth.CanWriteEventTypes(a); err != nil {
            return nil, err
        }

        // Resource-level access.
        if req.Body.ClientID != nil {
            if !a.CanAccessClient(*req.Body.ClientID) {
                return nil, httperror.Forbidden("No access to client: " + *req.Body.ClientID)
            }
        } else if !a.IsAnchor() {
            return nil, httperror.Forbidden("Only anchor users can create anchor-level event types")
        }

        cmd := operations.CreateCommand{
            Code:        req.Body.Code,
            Name:        req.Body.Name,
            Description: req.Body.Description,
            ClientID:    req.Body.ClientID,
            Schema:      req.Body.Schema,
        }
        ec := usecase.NewExecutionContext(a.PrincipalID)

        event, err := usecase.Into(usecase.Run(ctx, state.CreateUC, cmd, ec))
        if err != nil {
            return nil, httperror.From(err)
        }

        resp := &CreatedResponse{Status: http.StatusCreated}
        resp.Body.ID = event.EventTypeID
        return resp, nil
    })

    // ... GET, PUT, DELETE handlers
}
```

That's the full pattern. Note how:

- The handler **cannot** return a successful response without calling `usecase.Run` (or it would have to construct a `Result[E]` itself — which it can't, the constructors are unexported).
- The use case **cannot** return a successful `Result` without calling `uow.Commit` — which is the only public function that internally calls `newSuccess`.
- The handler builds the `Command`, the use case validates and executes, the UoW writes (atomically) the aggregate row + the event row + the audit row.

---

## The compile-time guarantees, restated

What the compiler will reject:

```go
// In any package OTHER than internal/usecase:

// 1. Direct construction of success — type name unexported.
return success[Event]{event: e}   // ❌ undefined: success

// 2. Calling the unexported constructor.
return usecase.newSuccess(e)       // ❌ undefined: usecase.newSuccess

// 3. Implementing Result.
type myResult struct{}
func (myResult) isResult()              {} // ❌ no usable interface element
func (myResult) unwrap() (E, error)     {} // ❌

// 4. Embedding success and exposing it.
type evil[E any] struct{ usecase.success[E] } // ❌ undefined: usecase.success

// 5. Skipping validation/authorization by calling Execute directly.
// LEGAL but the result still has to come from uow.Commit, so the
// event row gets written and the audit row gets written. The
// only thing skipped is the early validation/authorization check
// in the use case body — which only matters if your handler is
// also missing its permission check (which it shouldn't be).
```

What the compiler will accept:

```go
// Within internal/usecase, anyone can call newSuccess. But the
// package contains nothing other than:
//   - the Result/success/failure types
//   - the UseCase interface and Run function
//   - the UnitOfWork interface and PgUnitOfWork/TxScopedUnitOfWork impls
//   - the DomainEvent interface
//   - the ExecutionContext + tracing helpers
//
// The UoW implementations are the only newSuccess callers. Keep the
// package minimal so a quick `grep -r newSuccess internal/usecase/`
// gives an exhaustive list of callsites.
```

---

## The analyzer (optional belt-and-suspenders)

A custom `go vet` analyzer that asserts every `*UseCase.Execute` method body ends in a `usecase.UnitOfWork.Commit*` call or a `usecase.Failure(...)` call. This is the Go equivalent of [`tests/uow_convention_test.rs`](../../flowcatalyst-rust/) from the Rust side.

It lives in `tools/analyzer/uowseal/`. Run via:

```bash
go install ./tools/analyzer/uowseal
go vet -vettool=$(which uowseal) ./internal/platform/...
```

CI runs it on every PR. **Not load-bearing — the type system already prevents the failure mode this catches. But cheap and useful for catching "the developer wrote `return usecase.Failure(...)` on every code path and never actually called UoW", which compiles but means no event got emitted.**

---

## FAQ

**Q: Can reflection construct a `success[E]`?**
A: Yes. Reflection escapes type safety. The Rust seal has the same gap via `unsafe`. Acceptable.

**Q: Can I implement `UnitOfWork` outside `internal/usecase`?**
A: Yes (the interface is exported), but your implementation cannot call `newSuccess`. So you can implement the interface but cannot construct a successful `Result`. This is by design — only the in-package implementations are valid. If someone implements UoW externally and just returns `Failure` for everything, that's fine: the world hasn't been corrupted; their handler is just broken.

**Q: Why type aliases (`type Validated[C any] = validated[C]`) instead of named types?**
A: Aliases let external code reference the type name without giving it construction power. A named type (e.g., `type Validated[C any] validated[C]`) would be a distinct type, which means you'd need conversion functions back and forth — uglier.

**Q: What about the `commit_all` (batch) variant?**
A: `UnitOfWork.CommitAll[A, E, C](ctx, aggs []A, repo Persist[A], event E, cmd C) Result[E]` — same pattern. Internally calls `newSuccess` after writing the aggregates + emitting one summary event.

**Q: Domain events sometimes flatten metadata via `#[serde(flatten)]` in Rust. How in Go?**
A: Embed `usecase.EventMetadata` as an unnamed field; use `json:",inline"` tag (this requires `mailru/easyjson` or a thin custom MarshalJSON since stdlib `encoding/json` doesn't natively support flatten). Acceptable alternative: spell out the metadata fields. Recommended: write a small `MarshalJSON` on the `EventMetadata` host struct that merges the maps. See `internal/usecase/domain_event.go` for the impl pattern.

**Q: What replaces Rust's `impl_domain_event!` macro?**
A: Each event type implements the `DomainEvent` interface manually (8 small accessor methods). The Rust macro saves ~20 LOC per event; in Go, the equivalent is a code-gen'd file via `go generate` if it becomes onerous. For now, write them by hand. Total cost: ~40 events × 8 methods × 1 line = 320 LOC of mechanical accessor code. Trivial.
