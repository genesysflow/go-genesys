package query

// Paginator holds a page of results with Laravel-style pagination metadata.
type Paginator struct {
	Data        []map[string]any `json:"data"`
	Total       int64            `json:"total"`
	PerPage     int              `json:"per_page"`
	CurrentPage int              `json:"current_page"`
	LastPage    int              `json:"last_page"`
	From        int              `json:"from"`
	To          int              `json:"to"`
}

// SimplePaginator holds a page of results without a total count.
type SimplePaginator struct {
	Data        []map[string]any `json:"data"`
	PerPage     int              `json:"per_page"`
	CurrentPage int              `json:"current_page"`
	HasMore     bool             `json:"has_more"`
}

// Paginate runs the query for the given page (1-based) and returns
// the results with full pagination metadata (requires a COUNT query).
func (b *Builder) Paginate(page, perPage int) (*Paginator, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	total, err := b.Clone().Count()
	if err != nil {
		return nil, err
	}

	data, err := b.Clone().Limit(perPage).Offset((page - 1) * perPage).Get()
	if err != nil {
		return nil, err
	}

	lastPage := int((total + int64(perPage) - 1) / int64(perPage))
	if lastPage < 1 {
		lastPage = 1
	}

	from, to := 0, 0
	if len(data) > 0 {
		from = (page-1)*perPage + 1
		to = from + len(data) - 1
	}

	return &Paginator{
		Data:        data,
		Total:       total,
		PerPage:     perPage,
		CurrentPage: page,
		LastPage:    lastPage,
		From:        from,
		To:          to,
	}, nil
}

// SimplePaginate runs the query for the given page without counting the
// total: it fetches one extra row to detect whether more pages exist.
func (b *Builder) SimplePaginate(page, perPage int) (*SimplePaginator, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	data, err := b.Clone().Limit(perPage + 1).Offset((page - 1) * perPage).Get()
	if err != nil {
		return nil, err
	}

	hasMore := len(data) > perPage
	if hasMore {
		data = data[:perPage]
	}

	return &SimplePaginator{
		Data:        data,
		PerPage:     perPage,
		CurrentPage: page,
		HasMore:     hasMore,
	}, nil
}
