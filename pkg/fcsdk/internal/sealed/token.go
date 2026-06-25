// Package sealed provides a compile-time token that gates construction
// of [usecase.Result] success values. The Token type has an unexported
// field, so it cannot be constructed outside this package, and Go's
// internal/ import rule prevents anything outside pkg/fcsdk/ from
// importing this package at all.
//
// The combined effect: only packages under pkg/fcsdk/ can mint a Token,
// and only with a Token can a caller invoke usecase.Success. This is the
// Go analogue of the Rust SDK's `pub(in crate::usecase) fn success(...)`
// — compile-time enforced. Living under pkg/fcsdk/internal/ (rather than
// the repo-root internal/) is what scopes the seal to the SDK: platform
// code (internal/platform/...) cannot import it and so cannot mint a
// Success outside a UnitOfWork Commit*.
package sealed

// Token is the unforgeable witness that a caller is internal to the SDK.
// It is intentionally an empty struct with no exported fields and no
// way to construct one outside this package.
type Token struct{ _ struct{} }

// New produces a Token. Callable only by packages that can import
// pkg/fcsdk/internal/sealed — i.e. anything under pkg/fcsdk/. Application
// code is shut out at the import level.
func New() Token { return Token{} }
