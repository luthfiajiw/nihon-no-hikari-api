package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errInvalidPrerequisites = errors.New("prasyarat tidak valid")

// Exec mode infers PostgreSQL parameter types from Go values. pgx knows
// []string, but cannot infer an array type for Google's []uuid.UUID.
// Queries explicitly cast these strings to uuid[].
func prerequisiteIDs(required []uuid.UUID) []string {
	ids := make([]string, len(required))
	for i, id := range required {
		ids[i] = id.String()
	}
	return ids
}

func (v coreInput) prerequisites(kind string) []uuid.UUID {
	switch kind {
	case "module":
		return v.ModulePrerequisites
	case "lesson":
		return v.LessonPrerequisites
	default:
		return nil
	}
}

func validatePrerequisites(ctx context.Context, tx pgx.Tx, kind string, id uuid.UUID, required []uuid.UUID) error {
	if len(required) == 0 {
		return nil
	}
	// Only module and lesson can provide prerequisite IDs; never interpolate user input.
	if kind != "module" && kind != "lesson" {
		return errInvalidPrerequisites
	}
	seen := make(map[uuid.UUID]bool, len(required))
	for _, requiredID := range required {
		if requiredID == uuid.Nil || requiredID == id || seen[requiredID] {
			return fmt.Errorf("%w: pilih konten lain tanpa duplikasi", errInvalidPrerequisites)
		}
		seen[requiredID] = true
	}
	var count int
	ids := prerequisiteIDs(required)
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %ss WHERE id=ANY($1::uuid[])`, kind), ids).Scan(&count); err != nil {
		return err
	}
	if count != len(required) {
		return fmt.Errorf("%w: konten yang dipilih tidak ditemukan", errInvalidPrerequisites)
	}
	var cycle bool
	query := fmt.Sprintf(`WITH RECURSIVE dependencies(id) AS (
		SELECT unnest($2::uuid[])
		UNION
		SELECT p.required_%s_id FROM %s_prerequisites p JOIN dependencies d ON p.%s_id=d.id
	) SELECT EXISTS(SELECT 1 FROM dependencies WHERE id=$1)`, kind, kind, kind)
	if err := tx.QueryRow(ctx, query, id, ids).Scan(&cycle); err != nil {
		return err
	}
	if cycle {
		return fmt.Errorf("%w: hubungan prasyarat tidak boleh melingkar", errInvalidPrerequisites)
	}
	return nil
}

func savePrerequisites(ctx context.Context, tx pgx.Tx, kind string, id uuid.UUID, required []uuid.UUID) error {
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
