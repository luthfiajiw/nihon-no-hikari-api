package question

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type contentServiceStub struct {
	Service
	listPaginated func(context.Context, QuestionSetFilter) (ContentListResponse, error)
}

func (s *contentServiceStub) ListAdminQuestionSetsPaginated(ctx context.Context, filter QuestionSetFilter) (ContentListResponse, error) {
	if s.listPaginated != nil {
		return s.listPaginated(ctx, filter)
	}
	return ContentListResponse{}, nil
}

func TestAdminListForwardsKindFilter(t *testing.T) {
	var got QuestionSetFilter
	stub := &contentServiceStub{
		listPaginated: func(_ context.Context, filter QuestionSetFilter) (ContentListResponse, error) {
			got = filter
			return ContentListResponse{Success: true, Data: []json.RawMessage{}}, nil
		},
	}
	h := &Handler{service: stub}
	app := fiber.New()
	app.Get("/question-sets", h.adminList)

	res, err := app.Test(httptest.NewRequest("GET", "/question-sets?skill=writing&kind=%20practice%20&status=draft", nil))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("question-sets failed: err=%v status=%v", err, res.StatusCode)
	}
	if got.Skill != "writing" || got.Kind != "practice" || got.Status != "draft" {
		t.Fatalf("unexpected filter: %+v", got)
	}
}

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
