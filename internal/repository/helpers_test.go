package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"shop_project_be/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestIsRetryableTxError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"translated unique violation (open-debt race)", internalErr(fmt.Errorf("failed to create debt: %w", gorm.ErrDuplicatedKey)), true},
		{"raw unique violation", internalErr(&pgconn.PgError{Code: "23505"}), true},
		{"deadlock", fmt.Errorf("lock: %w", &pgconn.PgError{Code: "40P01"}), true},
		{"business duplicate is final", domain.Duplicate("invoice exists"), false},
		{"stock shortage is final", domain.Conflict("insufficient stock"), false},
		{"other driver failure", internalErr(errors.New("connection reset")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableTxError(tt.err); got != tt.want {
				t.Errorf("isRetryableTxError(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestTrimPage(t *testing.T) {
	key := func(n int) (time.Time, uuid.UUID) { return time.Unix(int64(n), 0), uuid.Nil }

	items, hasNext, next := trimPage([]int{1, 2, 3}, 2, key)
	if len(items) != 2 || !hasNext || next == nil || !next.AfterTime.Equal(time.Unix(2, 0)) {
		t.Errorf("probe row present: got items=%v hasNext=%v next=%+v; want [1 2], true, cursor at 2", items, hasNext, next)
	}

	items, hasNext, next = trimPage([]int{1, 2}, 2, key)
	if len(items) != 2 || hasNext || next != nil {
		t.Errorf("last page: got items=%v hasNext=%v next=%+v; want [1 2], false, nil", items, hasNext, next)
	}
}
