// Package apicommon holds reusable HTTP response shapes that match the
// Rust platform's wire format. The JSON shape of these types is part
// of the public API contract.
package apicommon

// CreatedResponse is the standard envelope returned on POST endpoints
// that create a single entity. Matches Rust CreatedResponse.
type CreatedResponse struct {
	ID string `json:"id"`
}

// PaginatedResponse is the standard offset-paginated envelope.
type PaginatedResponse[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// CursorResponse is the cursor-based pagination envelope used for
// high-volume firehose tables (events, dispatch jobs).
type CursorResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// SizeOnlyResponse is used by debug/firehose endpoints that take ?size= only.
type SizeOnlyResponse[T any] struct {
	Items []T `json:"items"`
}

// PaginationParams is the typical offset-based query.
type PaginationParams struct {
	Page     int `query:"page"`
	PageSize int `query:"pageSize"`
}

// Defaults applies the canonical defaults (page=1, pageSize=50).
func (p *PaginationParams) Defaults() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 200 {
		p.PageSize = 50
	}
}

// Offset returns the SQL offset for the current page/page-size.
func (p *PaginationParams) Offset() int { return (p.Page - 1) * p.PageSize }
