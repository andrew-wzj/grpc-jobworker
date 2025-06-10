package clientutil

import (
	"context"
	"time"

	"google.golang.org/grpc/metadata"
)

func CreateContext(username, password string) (context.Context, context.CancelFunc) {
	baseCtx := context.Background()
	ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)

	md := metadata.Pairs(
		"username", username,
		"password", password,
	)

	ctx = metadata.NewOutgoingContext(ctx, md)
	return ctx, cancel
}
