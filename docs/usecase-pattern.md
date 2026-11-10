# The use-case pattern: `usecaseop`

> The earlier version of this page described a `UseCase` + sealed `Result` +
> `internal/usecase` + `uow.Commit` design with type-state phases. That design
> was replaced by the `usecaseop` envelope and no longer exists in the code.
> This page is the current, short description. `CONVENTIONS.md` section 3 is
> the contributor contract, and `pkg/fcsdk/usecaseop/doc.go` is the reference.

Every state-changing business operation is a `usecaseop.Operation[C, E]`
(`pkg/fcsdk/usecaseop`). `usecaseop.Run` executes its phases in a fixed order:

| Phase | What it does |
|---|---|
| `Validate` | Command shape (presence, format, length). Pure; no database. |
| `Authorize` | Resource-level access. Required; `usecaseop.Public` declares an operation intentionally open. |
| `Execute` | Invariant checks (loads, business rules); builds the aggregate and the domain event and returns a `Plan`. |
| (Run) | Persists the aggregate, writes the domain event, any dispatch jobs the plan carries and the audit log, atomically in one transaction. |

## What is enforced, and how

- Validation and authorization always run, because `Run` runs them.
- An operation can reach the database only by returning a `Plan` from
  `Execute`. `Plan` is a sealed interface (its only method is unexported); the
  constructors are `usecaseop.Save`, `Delete`, `Emit`, `SaveAll` and `Sync`,
  and `Run` is its only consumer. So no path persists an aggregate without also
  writing the domain event and the audit row.
- `Authorize` cannot be left out silently. A missing one fails closed at runtime
  (`USECASE_MISCONFIGURED`), and the `uowseal` analyzer
  (`tools/analyzer/uowseal`, `make analyze`) flags an `Operation` literal that
  omits it at build time.
- Multi-aggregate commands use `usecaseop.TxOperation`.

## Shape

```go
func CreateConnection(repo *connection.Repository) usecaseop.Operation[CreateCommand, ConnectionCreated] {
    return usecaseop.Operation[CreateCommand, ConnectionCreated]{
        Name:      "CreateConnection",
        Validate:  func(ctx context.Context, cmd CreateCommand) error { /* ... */ },
        Authorize: func(ctx context.Context, cmd CreateCommand) error {
            return auth.RequireAnchor(auth.FromContext(ctx))
        },
        Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionCreated], error) {
            // invariant checks, build aggregate and event
            return usecaseop.Save(c, repo, event), nil
        },
    }
}
```

The handler runs it and gets the committed event back:

```go
ec := usecase.NewExecutionContext(ac.PrincipalID)
event, err := usecaseop.Run(ctx, s.UoW, operations.CreateConnection(s.Repo), cmd, ec)
```

Errors are `*usecase.Error` values (`Validation` 400, `Authorization` 403,
`NotFound` 404, `Conflict` and `BusinessRule` 409, `Internal` 500).

A plan may also carry dispatch jobs (`plan.Dispatch(jobs...)`): work the
platform delivers to a URL, written as `DISPATCH_JOB` outbox rows after the
event and before the audit row. See the `Plan` documentation in
`pkg/fcsdk/usecaseop/plan.go` and the Go SDK README.

The canonical example is `internal/platform/eventtype/operations/create.go`.
