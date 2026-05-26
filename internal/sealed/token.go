// Package sealed provides a compile-time token that gates construction
// of [usecase.Result] success values. The Token type has an unexported
// field, so it cannot be constructed outside this package, and Go's
// internal/ import rule prevents anything outside clients/go-sdk/ from
// importing this package at all.
//
// The combined effect: only packages under clients/go-sdk/ can mint a
// Token, and only with a Token can a caller invoke usecase.Success.
// This is the Go analogue of the Rust SDK's
// `pub(in crate::usecase) fn success(...)` — compile-time enforced.
package sealed

// Token is the unforgeable witness that a caller is internal to the SDK.
// It is intentionally an empty struct with no exported fields and no
// way to construct one outside this package.
type Token struct{ _ struct{} }

// New produces a Token. Callable only by packages that can import
// internal/sealed — i.e. anything under clients/go-sdk/. Application
// code is shut out at the import level.
func New() Token { return Token{} }
