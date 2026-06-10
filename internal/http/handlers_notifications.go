package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

type Notification struct {
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Payload interface{} `json:"payload,omitempty"`
}

func (h *Handler) StreamNotifications(c *fiber.Ctx) error {
	claims, ok := getClaims(c)
	if !ok {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		channelName := fmt.Sprintf("user_notifications:%s", claims.UserID)
		pubsub := h.redis.Subscribe(ctx, channelName)
		defer pubsub.Close()

		// Initial connection message
		initMsg := Notification{Type: "connected", Message: "listening for notifications"}
		b, _ := json.Marshal(initMsg)
		fmt.Fprintf(w, "data: %s\n\n", b)
		w.Flush()

		ch := pubsub.Channel()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
				if err := w.Flush(); err != nil {
					// Client disconnected
					return
				}
			case <-ticker.C:
				fmt.Fprintf(w, ": ping\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}
