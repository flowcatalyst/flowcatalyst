package usecase

// HasID is implemented by every aggregate so the UnitOfWork can identify
// the row being committed without reflection. Mirrors the Rust HasId
// trait.
//
// The method is named IDStr (not ID) so it doesn't collide with the
// typical exported `ID string` field on entity structs. Every entity
// provides:
//
//	type Foo struct { ID string `json:"id"` ... }
//	func (f *Foo) IDStr() string { return f.ID }
type HasID interface {
	IDStr() string
}
