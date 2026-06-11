package apicommon

// Out is the standard single-body huma output envelope, replacing the
// per-module listOutput/getOutput/createOutput wrappers. The wrapper
// type's name never reaches the OpenAPI spec — huma names response
// schemas from the Body field's named type — so sharing this generic is
// wire-neutral. Do NOT use it with an anonymous Body struct: huma falls
// back to the wrapper name for those, which would collide across
// modules and change the spec.
type Out[T any] struct {
	Body T
}

// In is the pure-body input envelope for create-style handlers, the
// request-side twin of Out. Inputs that combine a body with path or
// query params keep their module-local struct.
type In[T any] struct {
	Body T
}

// Empty is the field-less input/output for handlers that take no
// parameters or respond 204 No Content.
type Empty struct{}

// IDInput is the bare `{id}` path input shared by get/update/delete
// handlers. It carries no doc tag; handlers whose id parameter has a
// documented description keep their module-local input type (or accept
// a reviewed description-only api-bump when migrating).
type IDInput struct {
	ID string `path:"id"`
}

// OptStr maps an optional query filter to the repository convention:
// "" means "not supplied" and becomes nil.
func OptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// MapSlice converts entities to response DTOs, always returning a
// non-nil slice so an empty list serializes as [] rather than null.
func MapSlice[T, U any](in []T, f func(*T) U) []U {
	out := make([]U, 0, len(in))
	for i := range in {
		out = append(out, f(&in[i]))
	}
	return out
}
