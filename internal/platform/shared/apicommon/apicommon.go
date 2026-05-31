// Package apicommon holds reusable HTTP response shapes that match the
// Rust platform's wire format. The JSON shape of these types is part
// of the public API contract.
package apicommon

// CreatedResponse is the standard envelope returned on POST endpoints
// that create a single entity. Matches Rust CreatedResponse.
type CreatedResponse struct {
	ID string `json:"id"`
}

// StatusChangeResponse is the standard envelope for lifecycle endpoints
// (deactivate, send-password-reset, …) that return a human-readable
// message. Matches Rust's {"message": "..."} shape.
type StatusChangeResponse struct {
	Message string `json:"message"`
}

// SuccessResponse matches Rust's shared SuccessResponse {success, message?}
// returned by oauth-client activate/deactivate. `message` is omitted when empty.
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// OffsetPage is the offset-paginated envelope matching Rust fc-platform's
// PaginatedResponse<T>: `{data, page, size, total, total_pages}` with a
// 0-based page index.
type OffsetPage[T any] struct {
	Data       []T   `json:"data"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewOffsetPage builds an OffsetPage, computing total_pages and ensuring a
// non-nil Data slice so it serializes as `[]` rather than `null`.
func NewOffsetPage[T any](data []T, page, size int, total int64) OffsetPage[T] {
	if data == nil {
		data = []T{}
	}
	totalPages := 0
	if size > 0 {
		totalPages = int((total + int64(size) - 1) / int64(size))
	}
	return OffsetPage[T]{Data: data, Page: page, Size: size, Total: total, TotalPages: totalPages}
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

// PageQuery is the embeddable offset-pagination query, matching Rust's
// PaginationParams: `page` (0-based, default 0) and `size` (default 20),
// accepting the historical aliases `limit`/`pageSize`/`page_size` for size.
type PageQuery struct {
	Page          int `query:"page"`
	Size          int `query:"size"`
	Limit         int `query:"limit"`
	PageSizeCamel int `query:"pageSize"`
	PageSizeSnake int `query:"page_size"`
}

// PageIndex returns the resolved 0-based page index (default 0).
func (p PageQuery) PageIndex() int {
	if p.Page < 0 {
		return 0
	}
	return p.Page
}

// PageSizeVal returns the resolved page size (default 20), honoring the size
// aliases in priority order: size, limit, pageSize, page_size.
func (p PageQuery) PageSizeVal() int {
	for _, v := range []int{p.Size, p.Limit, p.PageSizeCamel, p.PageSizeSnake} {
		if v > 0 {
			return v
		}
	}
	return 20
}

// OffsetVal and LimitVal return the SQL offset/limit for the resolved page.
func (p PageQuery) OffsetVal() int64 { return int64(p.PageIndex()) * int64(p.PageSizeVal()) }
func (p PageQuery) LimitVal() int64  { return int64(p.PageSizeVal()) }
