package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReorderRequest includes every sibling, including unpublished and archived content.
type ReorderRequest struct {
	ParentID   uuid.UUID   `json:"parent_id" swaggertype:"string" format:"uuid"`
	OrderedIDs []uuid.UUID `json:"ordered_ids" swaggertype:"array,string"`
}

var errStaleOrder = errors.New("daftar konten telah berubah. Muat ulang dan coba lagi")

func parseReorder(kind string, raw []byte) (ReorderRequest, error) {
	var input ReorderRequest
	if kind != "module" && kind != "lesson" {
		return input, errors.New("jenis konten tidak valid")
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, errors.New("data urutan tidak valid")
	}
	if input.ParentID == uuid.Nil || input.OrderedIDs == nil || len(input.OrderedIDs) > 32768 {
		return input, errors.New("parent atau jumlah konten tidak valid")
	}
	seen := make(map[uuid.UUID]bool, len(input.OrderedIDs))
	for _, id := range input.OrderedIDs {
		if id == uuid.Nil || seen[id] {
			return input, errors.New("ID konten tidak valid atau berulang")
		}
		seen[id] = true
	}
	return input, nil
}

func reorderContent(ctx context.Context, tx pgx.Tx, kind string, input ReorderRequest) error {
	// Only these fixed identifiers are interpolated into SQL.
	var name, parentTable, parentField string
	switch kind {
	case "module":
		name, parentTable, parentField = "modules", "courses", "course_id"
	case "lesson":
		name, parentTable, parentField = "lessons", "modules", "module_id"
	default:
		return errors.New("jenis konten tidak valid")
	}
	var parentID uuid.UUID
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE id=$1 FOR UPDATE`, parentTable), input.ParentID).Scan(&parentID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE %s=$1 ORDER BY id FOR UPDATE`, name, parentField), input.ParentID)
	if err != nil {
		return err
	}
	siblings, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	if len(siblings) != len(input.OrderedIDs) {
		return errStaleOrder
	}
	requested := make(map[uuid.UUID]bool, len(input.OrderedIDs))
	for _, id := range input.OrderedIDs {
		requested[id] = true
	}
	for _, id := range siblings {
		if !requested[id] {
			return errStaleOrder
		}
	}
	ids := prerequisiteIDs(input.OrderedIDs)
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE public.%s t SET order_index=(ordering.position-1)::smallint, updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE t.id=ordering.id AND t.%s=$2`, name, parentField), ids, input.ParentID); err != nil {
		return err
	}
	// A later publish must keep the new order without publishing or discarding other edits.
	// Drafts destined for another parent retain their intended destination order.
	_, err = tx.Exec(ctx, fmt.Sprintf(`
		UPDATE public.admin_drafts d
		SET payload=jsonb_set(d.payload, '{order_index}', to_jsonb((ordering.position-1)::smallint)), updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE d.resource_type=$2 AND d.resource_id=ordering.id AND d.payload->>'%s'=$3`, parentField), ids, kind, input.ParentID.String())
	return err
}

// reorder godoc
// @Summary Simpan urutan module atau lesson
// @Description Memerlukan role Admin. ordered_ids wajib memuat seluruh anak parent, termasuk draft dan arsip. Module diurutkan dalam course; lesson dalam module. Urutan langsung berlaku tanpa publish. Hanya order_index pada draft dengan parent yang sama yang disinkronkan; perubahan materi lainnya dipertahankan. Seluruh perubahan disimpan dalam satu transaksi.
// @Tags Admin Content
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis konten" Enums(module,lesson)
// @Param request body ReorderRequest true "Parent dan seluruh ID sesuai urutan baru"
// @Success 200 {object} StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/reorder [post]
func (h *Handler) reorder(c fiber.Ctx) error {
	kind := c.Params("kind")
	input, err := parseReorder(kind, c.Body())
	if err != nil {
		return bad(c, err.Error())
	}
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	if err := reorderContent(c.Context(), tx, kind, input); err != nil {
		if errors.Is(err, errStaleOrder) {
			return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true})
}
