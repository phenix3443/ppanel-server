package orm

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// CommaSeparatedContains filters comma-separated string columns, such as "1,2,3".
func CommaSeparatedContains(field string, values []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		condition, args := CommaSeparatedContainsCondition(db, field, values)
		if condition == "" {
			return db
		}
		return db.Where(condition, args...)
	}
}

// CommaSeparatedContainsCondition returns the dialect-aware SQL fragment used
// when multiple CSV predicates must be combined in one surrounding OR query.
func CommaSeparatedContainsCondition(db *gorm.DB, field string, values []string) (string, []interface{}) {
	values = removeEmpty(values)
	if len(values) == 0 {
		return "", nil
	}
	conds := make([]string, len(values))
	args := make([]interface{}, len(values))
	if db.Dialector.Name() == DriverMySQL {
		for i, v := range values {
			conds[i] = "FIND_IN_SET(?, " + field + ")"
			args[i] = v
		}
	} else {
		for i, v := range values {
			conds[i] = "(',' || COALESCE(" + field + ", '') || ',') LIKE ?"
			args[i] = "%," + v + ",%"
		}
	}
	return "(" + strings.Join(conds, " OR ") + ")", args
}

func removeEmpty(values []string) []string {
	list := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			list = append(list, value)
		}
	}
	return list
}

func TextColumnExpr(db *gorm.DB, field string) string {
	if db.Dialector.Name() == DriverPostgres {
		return fmt.Sprintf("CAST(%s AS TEXT)", field)
	}
	return fmt.Sprintf("CAST(%s AS CHAR)", field)
}
