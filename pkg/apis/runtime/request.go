package runtime

import (
	"entgo.io/ent/dialect/sql"
)

// RequestCollection holds the requesting data of collection,
// including querying, sorting, extracting and pagination.
type RequestCollection[Q, S ~func(*sql.Selector)] struct {
	RequestPagination `query:",inline"`
}

// RequestPagination holds the requesting pagination data.
type RequestPagination struct {
	// Page specifies the page number for querying,
	// i.e. /v1/repositories?page=1&perPage=10.
	Page int `query:"page,default=1"`

	// PerPage specifies the page size for querying,
	// i.e. /v1/repositories?page=1&perPage=10.
	PerPage int `query:"perPage,default=100"`
}

// Limit returns the limit of paging.
func (r RequestPagination) Limit() int {
	limit := r.PerPage
	if limit <= 0 {
		limit = 100
	}

	return limit
}

// Offset returns the offset of paging.
func (r RequestPagination) Offset() int {
	offset := r.Limit() * (r.Page - 1)
	if offset < 0 {
		offset = 0
	}

	return offset
}

// Paging returns the limit and offset of paging,
// returns false if there is no pagination requesting.
func (r RequestPagination) Paging() (limit, offset int, request bool) {
	request = r.Page > 0
	if !request {
		return
	}
	limit = r.Limit()
	offset = r.Offset()

	return
}
