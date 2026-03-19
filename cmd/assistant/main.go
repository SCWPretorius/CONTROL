package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SCWPretorius/CONTROL/internal/app"
	"github.com/SCWPretorius/CONTROL/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "assistant: ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Fatalf("run assistant: %v", err)
	}
}
