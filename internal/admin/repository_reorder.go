package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *dbRepository) LockSiblings(ctx context.Context, kind string, parent uuid.UUID) ([]uuid.UUID, error) {
	// Only these fixed identifiers are interpolated into SQL.
	var name, parentTable, parentField string
	switch kind {
	case "module":
		name, parentTable, parentField = "modules", "courses", "course_id"
	case "lesson":
		name, parentTable, parentField = "lessons", "modules", "module_id"
	default:
		return nil, inputMessage("jenis konten tidak valid")
	}
	var parentID uuid.UUID
	if err := r.QueryRow(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE id=$1 FOR UPDATE`, parentTable), parent).Scan(&parentID); err != nil {
		return nil, err
	}
	rows, err := r.Query(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE %s=$1 ORDER BY id FOR UPDATE`, name, parentField), parent)
	if err != nil {
		return nil, err
	}
	siblings, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return siblings, nil
}

func (r *dbRepository) ApplyOrder(ctx context.Context, kind string, input ReorderRequest) error {
	var name, parentField string
	switch kind {
	case "module":
		name, parentField = "modules", "course_id"
	case "lesson":
		name, parentField = "lessons", "module_id"
	default:
		return inputMessage("jenis konten tidak valid")
	}
	ids := prerequisiteIDs(input.OrderedIDs)
	if _, err := r.Exec(ctx, fmt.Sprintf(`
		UPDATE public.%s t SET order_index=(ordering.position-1)::smallint, updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE t.id=ordering.id AND t.%s=$2`, name, parentField), ids, input.ParentID); err != nil {
		return err
	}
	// A later publish must keep the new order without publishing or discarding other edits.
	// Drafts destined for another parent retain their intended destination order.
	_, err := r.Exec(ctx, fmt.Sprintf(`
		UPDATE public.admin_drafts d
		SET payload=jsonb_set(d.payload, '{order_index}', to_jsonb((ordering.position-1)::smallint)), updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE d.resource_type=$2 AND d.resource_id=ordering.id AND d.payload->>'%s'=$3`, parentField), ids, kind, input.ParentID.String())
	return err
}
