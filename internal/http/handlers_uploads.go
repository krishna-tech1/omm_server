package http

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type presignUploadRequest struct {
	Purpose       string `json:"purpose" validate:"required"`
	ContentType   string `json:"content_type" validate:"required"`
	FileExtension string `json:"file_extension"`
}

type presignUploadResponse struct {
	UploadURL string            `json:"upload_url"`
	PublicURL string            `json:"public_url"`
	ObjectKey string            `json:"object_key"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

const presignTTL = 15 * time.Minute

func (h *Handler) PresignUpload(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return h.respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if h.r2InitErr != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "uploads not configured")
	}

	if h.r2Presigner == nil || strings.TrimSpace(h.cfg.R2Bucket) == "" || strings.TrimSpace(h.cfg.R2PublicBaseURL) == "" {
		return h.respondError(c, fiber.StatusServiceUnavailable, "uploads not configured")
	}

	var req presignUploadRequest
	if err := h.parseBody(c, &req); err != nil {
		return h.respondError(c, fiber.StatusBadRequest, "invalid request")
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	prefix, err := uploadPrefixForPurpose(purpose)
	if err != nil {
		return h.respondError(c, fiber.StatusBadRequest, err.Error())
	}

	contentType := strings.ToLower(strings.TrimSpace(req.ContentType))
	if !strings.HasPrefix(contentType, "image/") {
		return h.respondError(c, fiber.StatusBadRequest, "invalid content type")
	}

	ext := strings.ToLower(strings.TrimSpace(req.FileExtension))
	ext = strings.TrimPrefix(ext, ".")
	if ext != "" && !isSafeExtension(ext) {
		return h.respondError(c, fiber.StatusBadRequest, "invalid file extension")
	}

	objectKey := fmt.Sprintf("%s/%s/%s", prefix, claims.UserID.String(), uuid.New().String())
	if ext != "" {
		objectKey = objectKey + "." + ext
	}

	ctx, cancel := h.requestContext()
	defer cancel()

	result, err := h.r2Presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(h.cfg.R2Bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = presignTTL
	})
	if err != nil {
		return h.respondError(c, fiber.StatusInternalServerError, "failed to create upload url")
	}

	publicBase := strings.TrimRight(h.cfg.R2PublicBaseURL, "/")
	return c.JSON(presignUploadResponse{
		UploadURL: result.URL,
		PublicURL: publicBase + "/" + objectKey,
		ObjectKey: objectKey,
		Method:    result.Method,
		Headers:   flattenHeaders(result.SignedHeader),
		ExpiresAt: time.Now().Add(presignTTL),
	})
}

func uploadPrefixForPurpose(purpose string) (string, error) {
	switch purpose {
	case "avatar":
		return "avatars", nil
	case "merchant_logo":
		return "merchant-logos", nil
	case "reward_image":
		return "reward-images", nil
	default:
		return "", fmt.Errorf("unsupported purpose")
	}
}

func isSafeExtension(ext string) bool {
	if len(ext) < 1 || len(ext) > 10 {
		return false
	}
	for _, r := range ext {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	flattened := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		flattened[key] = values[0]
	}

	return flattened
}
