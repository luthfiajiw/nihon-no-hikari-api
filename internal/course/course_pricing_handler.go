package course

import (
	"github.com/gofiber/fiber/v3"
)

// pricingList godoc
// @Summary Daftar paket harga publik
// @Description Mengambil daftar level/paket belajar untuk halaman harga/landing page publik tanpa autentikasi.
// @Tags Pricing
// @Produce json
// @Success 200 {object} model.PricingListResponse
// @Failure 500 {object} utils.ErrorRes
// @Router /pricing [get]
func (h *Handler) pricingList(c fiber.Ctx) error {
	response, err := h.service.PricingList(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// pricingDetail godoc
// @Summary Detail paket harga dan modul
// @Description Mengambil detail paket belajar beserta daftar modul yang tersedia untuk level tersebut tanpa autentikasi.
// @Tags Pricing
// @Produce json
// @Param code path string true "Kode level (misal N5)"
// @Success 200 {object} model.PricingDetailResponse
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /pricing/{code} [get]
func (h *Handler) pricingDetail(c fiber.Ctx) error {
	code := c.Params("code")
	response, err := h.service.PricingDetail(c.Context(), code)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}
