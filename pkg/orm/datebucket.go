package orm

import (
	"fmt"

	"gorm.io/gorm"
)

// DateBucketExpr renders the dialect-specific SQL that buckets a timestamp
// column by day or by month ("YYYY-MM-DD" / "YYYY-MM"), for GROUP BY
// statistics queries.
func DateBucketExpr(db *gorm.DB, column, bucket string) string {
	if db.Dialector.Name() == "postgres" {
		if bucket == "month" {
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM')", column)
		}
		return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM-DD')", column)
	}
	if bucket == "month" {
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m')", column)
	}
	return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%d')", column)
}
