package http

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type adminStatsResponse struct {
	TotalUsers         int64   `json:"total_users"`
	TotalMerchants     int64   `json:"total_merchants"`
	TotalDistanceMiles float64 `json:"total_distance_miles"`
	PremiumUsers       int64   `json:"premium_users"`
	MRR                float64 `json:"mrr"`
}

func (h *Handler) AdminStats(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	stats, err := h.db.AdminStats(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load stats")
	}

	mrrCents, err := h.db.GetMRR(ctx)
	if err != nil {
		mrrCents = 0 // Ignore error, just default to 0
	}

	return c.JSON(adminStatsResponse{
		TotalUsers:         stats.TotalUsers,
		TotalMerchants:     stats.TotalMerchants,
		TotalDistanceMiles: stats.TotalDistanceMiles,
		PremiumUsers:       stats.PremiumUsers,
		MRR:                float64(mrrCents) / 100.0,
	})
}

func (h *Handler) ListAllMerchants(c *fiber.Ctx) error {
	ctx, cancel := h.requestContext()
	defer cancel()

	merchants, err := h.db.ListAllMerchantsWithAnalytics(ctx)
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to load merchants")
	}

	response := make([]fiber.Map, 0, len(merchants))
	for _, m := range merchants {
		response = append(response, fiber.Map{
			"id":                 m.ID,
			"owner_user_id":      m.OwnerUserID,
			"name":               m.Name,
			"category":           m.Category,
			"address_lat":        m.AddressLat,
			"address_lng":        m.AddressLng,
			"logo_url":           m.LogoUrl,
			"description":        m.Description,
			"total_redemptions":  m.TotalRedemptions,
			"weekly_redemptions": m.WeeklyRedemptions,
		})
	}

	return c.JSON(response)
}

type banMerchantRequest struct {
	Remark string `json:"remark"`
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

	var req banMerchantRequest
	_ = h.parseBody(c, &req)

	msg := "Your merchant account has been suspended due to policy violations."
	if req.Remark != "" {
		msg += " Remark: " + req.Remark
	}

	ownerNotice := Notification{
		Type:    "merchant_banned",
		Message: msg,
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

	h.redis.Set(ctx, "banned:"+merchant.OwnerUserID.String(), "1", 0)

	// Send notification to the merchant
	ownerUser, err := h.db.GetUserByID(ctx, merchant.OwnerUserID)
	if err == nil {
		log.Printf("NOTIFICATION: Sending ban notice to merchant owner %s", ownerUser.Phone)
	}

	// Send a force_logout notification to clear their active session
	logoutNotice := Notification{
		Type: "force_logout",
	}
	logoutBytes, _ := json.Marshal(logoutNotice)
	h.redis.Publish(ctx, fmt.Sprintf("user_notifications:%s", merchant.OwnerUserID), string(logoutBytes))

	return c.SendStatus(fiber.StatusOK)
}
