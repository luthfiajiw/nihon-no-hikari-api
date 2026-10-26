package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func prerequisiteIDs(required []uuid.UUID) []string {
	ids := make([]string, len(required))
	for i, id := range required {
		ids[i] = id.String()
	}
	return ids
}

func savePrerequisites(ctx context.Context, tx dbQuerier, kind string, id uuid.UUID, required []uuid.UUID) error {
	// Omitted/null keeps existing relations for older clients and drafts; [] clears them.
	if required == nil {
		return nil
	}
	if kind != "module" && kind != "lesson" {
		return errInvalidPrerequisites
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s_prerequisites WHERE %s_id=$1`, kind, kind), id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s_prerequisites(%s_id,required_%s_id) SELECT $1, unnest($2::uuid[])`, kind, kind, kind), id, prerequisiteIDs(required))
	return err
}

func (r *dbRepository) PrerequisiteState(ctx context.Context, kind string, id uuid.UUID, required []uuid.UUID) (int, bool, error) {
	var count int
	ids := prerequisiteIDs(required)
	if err := r.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %ss WHERE id=ANY($1::uuid[])`, kind), ids).Scan(&count); err != nil {
		return 0, false, err
	}
	if count != len(required) {
		return count, false, nil
	}
	var cycle bool
	query := fmt.Sprintf(`WITH RECURSIVE dependencies(id) AS (
		SELECT unnest($2::uuid[])
		UNION
		SELECT p.required_%s_id FROM %s_prerequisites p JOIN dependencies d ON p.%s_id=d.id
	) SELECT EXISTS(SELECT 1 FROM dependencies WHERE id=$1)`, kind, kind, kind)
	if err := r.QueryRow(ctx, query, id, ids).Scan(&cycle); err != nil {
		return 0, false, err
	}
	return count, cycle, nil
}
