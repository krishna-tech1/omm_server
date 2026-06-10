package http

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func (h *Handler) AdminStats(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	stats, err := h.db.AdminStats(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load stats")
	}

	return c.JSON(stats)
}

func (h *Handler) ListAllMerchants(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	merchants, err := h.db.ListAllMerchants(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load merchants")
	}

	response := make([]merchantResponse, 0, len(merchants))
	for _, m := range merchants {
		response = append(response, mapMerchant(m))
	}

	return c.JSON(response)
}

func (h *Handler) BanMerchant(c *fiber.Ctx) error {
	merchantID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid merchant id")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	// Get the merchant to find the owner_user_id
	merchant, err := h.db.GetMerchantByID(ctx, merchantID)
	if err != nil {
		return h.respondError(c, fiber.StatusNotFound, "merchant not found")
	}

	// Get affected users to notify them
	affectedPhones, err := h.db.GetAffectedUsersByMerchantBan(ctx, merchantID)
	if err != nil {
		log.Printf("Failed to get affected users: %v", err)
	}

	ownerNotice := Notification{
		Type:    "merchant_banned",
		Message: "Your merchant account has been suspended due to policy violations.",
	}
	ownerBytes, _ := json.Marshal(ownerNotice)
	h.redis.Publish(ctx, fmt.Sprintf("user_notifications:%s", merchant.OwnerUserID), string(ownerBytes))

	userNotice := Notification{
		Type:    "challenge_cancelled",
		Message: "A challenge you were participating in has been cancelled due to a merchant policy violation. We apologize for the inconvenience.",
	}
	userBytes, _ := json.Marshal(userNotice)

	for _, phone := range affectedPhones {
		user, err := h.db.GetUserByPhone(ctx, phone)
		if err == nil {
			h.redis.Publish(ctx, fmt.Sprintf("user_notifications:%s", user.ID), string(userBytes))
		}
	}

	// Delete merchant's challenges
	if err := h.db.CancelMerchantChallenges(ctx, merchantID); err != nil {
		log.Printf("Failed to cancel merchant challenges: %v", err)
		return h.respondError(c, fiber.StatusInternalServerError, "failed to cancel challenges")
	}

	// Ban the merchant's owner
	if err := h.db.BanUser(ctx, merchant.OwnerUserID); err != nil {
		log.Printf("Failed to ban merchant owner: %v", err)
		return h.respondError(c, fiber.StatusInternalServerError, "failed to ban merchant")
	}

	// Send notification to the merchant
	ownerUser, err := h.db.GetUserByID(ctx, merchant.OwnerUserID)
	if err == nil {
		log.Printf("NOTIFICATION: Sending ban notice to merchant owner %s", ownerUser.Phone)
	}

	// To logout the banned merchant, we could invalidate sessions if we had a token blocklist.
	// We'll rely on the middleware checking `is_banned` flag which we should add.

	return c.SendStatus(fiber.StatusOK)
}
