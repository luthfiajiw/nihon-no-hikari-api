package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseErrorsUseCodesAndKeepOriginalCause(t *testing.T) {
	for _, tc := range []struct{ source, want error }{
		{pgx.ErrNoRows, ErrNotFound},
		{&pgconn.PgError{Code: "23505"}, ErrConflict},
		{&pgconn.PgError{Code: "23503"}, ErrInvalidRelation},
		{&pgconn.PgError{Code: "23514"}, ErrInvalidRelation},
	} {
		err := databaseError(fmt.Errorf("operation: %w", tc.source))
		if !errors.Is(err, tc.want) || !errors.Is(err, tc.source) {
			t.Fatalf("%v", err)
		}
	}
	ordinary := errors.New("SQLSTATE 23505")
	if err := databaseError(ordinary); err != ordinary || errors.Is(err, ErrConflict) {
		t.Fatalf("parsed error text: %v", err)
	}
}

func TestResponseEnvelopesPreserveNullAndOmittedFields(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{PublishResponse{Success: true}, `{"success":true}`},
		{ContentDetailResponse{Success: true, Data: json.RawMessage(`{}`)}, `{"success":true,"data":{},"draft":null}`},
		{ContentListResponse{Success: true, Data: []json.RawMessage{}}, `{"success":true,"data":[]}`},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil || string(raw) != tc.want {
			t.Fatalf("%s != %s (%v)", raw, tc.want, err)
		}
	}
}
