package http

import (
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/paymentintent"
	"github.com/stripe/stripe-go/v78/webhook"
)

type createIntentRequest struct {
	Amount   int64  `json:"amount" validate:"required"` // Amount in smallest currency unit (cents)
	Currency string `json:"currency" validate:"required"`
}

func (h *Handler) CreateStripeIntent(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if h.cfg.StripeSecretKey == "" {
		return h.respondError(c, fiber.StatusInternalServerError, "payments not configured")
	}

	var req createIntentRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	stripe.Key = h.cfg.StripeSecretKey

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(req.Amount),
		Currency: stripe.String(req.Currency),
	}
	params.AddMetadata("user_id", claims.UserID.String())

	pi, err := paymentintent.New(params)
	if err != nil {
		log.Printf("Stripe intent creation failed: %v", err)
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create payment intent")
	}

	return c.JSON(fiber.Map{
		"client_secret": pi.ClientSecret,
		"intent_id":     pi.ID,
	})
}

func (h *Handler) StripeWebhook(c *fiber.Ctx) error {
	if h.cfg.StripeSecretKey == "" || h.cfg.StripeWebhookSecret == "" {
		return h.respondError(c, fiber.StatusInternalServerError, "payments not configured")
	}

	signature := c.Get("Stripe-Signature")
	if signature == "" {
		return h.respondError(c, fiber.StatusUnauthorized, "missing signature")
	}

	body := c.Body()

	event, err := webhook.ConstructEvent(body, signature, h.cfg.StripeWebhookSecret)
	if err != nil {
		log.Printf("Stripe webhook verification failed: %v", err)
		return h.respondError(c, fiber.StatusBadRequest, "invalid signature")
	}

	if event.Type == "payment_intent.succeeded" {
		var pi stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &pi)
		if err != nil {
			return h.respondError(c, fiber.StatusBadRequest, "invalid payload")
		}

		userIDStr := pi.Metadata["user_id"]
		if userIDStr != "" {
			userID, err := uuid.Parse(userIDStr)
			if err == nil {
				ctx, cancel := h.requestContext()
				defer cancel()

				err = h.db.UpgradeUserToPremium(ctx, userID)
				if err != nil {
					log.Printf("Failed to upgrade user %v to premium: %v", userID, err)
				} else {
					log.Printf("Successfully upgraded user %v to premium via Stripe", userID)
				}
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
