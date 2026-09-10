package repository

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// NormalizePage clamps pagination inputs; module repo implementations share it.
func NormalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return page, size
}

// NormalizePageFloor clamps pagination inputs without a minimum page size.
func NormalizePageFloor(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = DefaultPageSize
	}
	return page, size
}
