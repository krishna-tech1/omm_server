package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	razorpay "github.com/razorpay/razorpay-go"
)

type createOrderRequest struct {
	Amount   int    `json:"amount" validate:"required"` // Amount in smallest currency unit (e.g., paise)
	Currency string `json:"currency" validate:"required"`
}

func (h *Handler) CreateRazorpayOrder(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if h.cfg.RazorpayKeyID == "" || h.cfg.RazorpayKeySecret == "" {
		return h.respondError(c, fiber.StatusInternalServerError, "payments not configured")
	}

	var req createOrderRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	client := razorpay.NewClient(h.cfg.RazorpayKeyID, h.cfg.RazorpayKeySecret)

	data := map[string]interface{}{
		"amount":   req.Amount,
		"currency": req.Currency,
		"receipt":  "rcptid_" + claims.UserID.String()[:8],
		"notes": map[string]interface{}{
			"user_id": claims.UserID.String(),
		},
	}

	body, err := client.Order.Create(data, nil)
	if err != nil {
		log.Printf("Razorpay order creation failed: %v", err)
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create payment order")
	}

	return c.JSON(body)
}

func (h *Handler) RazorpayWebhook(c *fiber.Ctx) error {
	if h.cfg.RazorpayKeyID == "" || h.cfg.RazorpayKeySecret == "" {
		return h.respondError(c, fiber.StatusInternalServerError, "payments not configured")
	}

	signature := c.Get("X-Razorpay-Signature")
	if signature == "" {
		return h.respondError(c, fiber.StatusUnauthorized, "missing signature")
	}

	body := c.Body()

	// Verify webhook signature
	mac := hmac.New(sha256.New, []byte(h.cfg.RazorpayKeySecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return h.respondError(c, fiber.StatusUnauthorized, "invalid signature")
	}

	var payload struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					Status string `json:"status"`
					Notes  struct {
						UserID string `json:"user_id"`
					} `json:"notes"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid payload")
	}

	if payload.Event == "payment.captured" {
		userIDStr := payload.Payload.Payment.Entity.Notes.UserID
		if userIDStr != "" {
			userID, err := uuid.Parse(userIDStr)
			if err == nil {
				ctx, cancel := h.requestContext()
				defer cancel()

				err = h.db.UpgradeUserToPremium(ctx, userID)
				if err != nil {
					log.Printf("Failed to upgrade user %v to premium: %v", userID, err)
				} else {
					log.Printf("Successfully upgraded user %v to premium via Razorpay", userID)
				}
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
