package http

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	verify "github.com/twilio/twilio-go/rest/verify/v2"

	"one-more-mile/server/internal/http/middleware"
	db "one-more-mile/server/internal/sqlc"
)

type otpRequest struct {
	Phone string `json:"phone" validate:"required"`
}

func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var req otpRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	user, err := h.db.GetUserByPhone(ctx, req.Phone)
	if err == nil {
		if user.IsBanned {
			return h.respondError(c, fiber.StatusForbidden, "user is banned")
		}
	} else if err != pgx.ErrNoRows {
		log.Printf("failed to check user ban status: %v", err)
		return h.respondError(c, fiber.StatusInternalServerError, "internal error")
	}

	if h.twilioEnabled() {
		if err := h.sendTwilioOTP(req.Phone); err != nil {
			return h.respondError(c, fiber.StatusBadRequest, "failed to send otp")
		}
	}

	return c.JSON(fiber.Map{"message": "success"})
}

type verifyOTPRequest struct {
	Phone string `json:"phone" validate:"required"`
	Code  string `json:"code" validate:"required"`
}

type verifyOTPResponse struct {
	User  userResponse `json:"user"`
	Token string       `json:"token"`
}

func (h *Handler) VerifyOTP(c *fiber.Ctx) error {
	var req verifyOTPRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	if h.twilioEnabled() && !(h.cfg.Env == "dev" && req.Code == "123456") {
		if err := h.verifyTwilioOTP(req.Phone, req.Code); err != nil {
			return h.respondError(c, fiber.StatusUnauthorized, "invalid otp")
		}
	}

	user, err := h.db.GetUserByPhone(ctx, req.Phone)
	if err != nil {
		if err == pgx.ErrNoRows {
			userRole := db.UserRoleUser
			if h.isEmployeePhone(ctx, req.Phone) {
				userRole = db.UserRoleEmployee
			}
			user, err = h.db.CreateUser(ctx, db.CreateUserParams{
				ID:    uuid.New(),
				Phone: req.Phone,
				Role:  userRole,
			})
			if err != nil {
				log.Printf("failed to create user: %v", err)
				return h.respondError(c, fiber.StatusInternalServerError, "failed to create user")
			}
		} else {
			log.Printf("failed to load user: %v", err)
			return h.respondError(c, fiber.StatusInternalServerError, "failed to load user")
		}
	}

	if user.IsBanned {
		return h.respondError(c, fiber.StatusForbidden, "user is banned")
	}

	if h.isEmployeePhone(ctx, req.Phone) && user.Role != db.UserRoleEmployee {
		if err := h.db.UpdateUserRole(ctx, db.UpdateUserRoleParams{
			ID:   user.ID,
			Role: db.UserRoleEmployee,
		}); err != nil {
			return h.respondError(c, fiber.StatusInternalServerError, "failed to update user role")
		}
		user.Role = db.UserRoleEmployee
	}

	token, err := middleware.GenerateToken(h.cfg, user.ID, string(user.Role))
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to issue token")
	}

	return c.JSON(verifyOTPResponse{
		User:  mapUser(user),
		Token: token,
	})
}

func (h *Handler) isEmployeePhone(ctx context.Context, phone string) bool {
	_, err := h.db.GetEmployeeByPhone(ctx, phone)
	return err == nil
}

func (h *Handler) twilioEnabled() bool {
	return h.twilioClient != nil && strings.TrimSpace(h.twilioVerifyServiceID) != ""
}

func (h *Handler) sendTwilioOTP(phone string) error {
	params := &verify.CreateVerificationParams{}
	params.SetTo(phone)
	params.SetChannel("sms")

	resp, err := h.twilioClient.VerifyV2.CreateVerification(h.twilioVerifyServiceID, params)
	if err != nil {
		return err
	}
	if resp == nil || resp.Status == nil {
		return errors.New("twilio verification failed")
	}

	status := strings.ToLower(*resp.Status)
	if status != "pending" && status != "approved" {
		return errors.New("twilio verification failed")
	}

	return nil
}

func (h *Handler) verifyTwilioOTP(phone, code string) error {
	params := &verify.CreateVerificationCheckParams{}
	params.SetTo(phone)
	params.SetCode(code)

	resp, err := h.twilioClient.VerifyV2.CreateVerificationCheck(h.twilioVerifyServiceID, params)
	if err != nil {
		return err
	}
	if resp == nil || resp.Status == nil {
		return errors.New("invalid otp")
	}

	status := strings.ToLower(*resp.Status)
	if status != "approved" {
		return errors.New("invalid otp")
	}

	return nil
}
