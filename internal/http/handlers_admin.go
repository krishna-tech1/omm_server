package http

import "github.com/gofiber/fiber/v2"

func (h *Handler) AdminStats(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	stats, err := h.db.AdminStats(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load stats")
	}

	return c.JSON(stats)
}
