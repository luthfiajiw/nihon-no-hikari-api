package admin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPrerequisiteParametersEncodeInExecMode(t *testing.T) {
	id := uuid.New()
	for _, required := range [][]uuid.UUID{{}, {id}} {
		ids := prerequisiteIDs(required)
		if len(ids) != len(required) || (len(ids) > 0 && ids[0] != id.String()) {
			t.Fatalf("converted prerequisites changed IDs: %v", ids)
		}
		// A nil statement description reproduces the pool's QueryExecModeExec,
		// where []uuid.UUID fails even when the SQL explicitly casts to uuid[].
		var query pgx.ExtendedQueryBuilder
		if err := query.Build(pgtype.NewMap(), nil, []any{id, ids}); err != nil {
			t.Fatalf("cannot encode prerequisite parameters: %v", err)
		}
	}
}
