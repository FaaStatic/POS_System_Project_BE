package repository

import (
	"fmt"
	"strings"
	"time"

	"shop_project_be/internal/constant/paginated"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func pageParams(limit int, order string) (int, string) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if strings.EqualFold(order, "ASC") {
		return limit, "ASC"
	}
	return limit, "DESC"
}

func keysetPage(q *gorm.DB, prefix string, limit int, order string, cursor *paginated.CursorMeta) *gorm.DB {
	if cursor != nil {
		op := "<"
		if order == "ASC" {
			op = ">"
		}
		q = q.Where(fmt.Sprintf("(%[1]screated_at, %[1]sid) %[2]s (?, ?)", prefix, op), cursor.AfterTime, cursor.AfterID)
	}
	return q.Order(fmt.Sprintf("%[1]screated_at %[2]s, %[1]sid %[2]s", prefix, order)).Limit(limit + 1)
}

func trimPage[T any](items []T, limit int, key func(T) (time.Time, uuid.UUID)) ([]T, bool, *paginated.CursorMeta) {
	if len(items) <= limit {
		return items, false, nil
	}
	items = items[:limit]
	afterTime, afterID := key(items[limit-1])
	return items, true, &paginated.CursorMeta{AfterTime: afterTime, AfterID: afterID}
}
