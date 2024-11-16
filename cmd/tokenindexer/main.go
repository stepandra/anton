package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/stepandra/anton/internal/app/tokenindexer"
    "github.com/stepandra/anton/internal/core/repository"
)

func main() {
    configPath := flag.String("config", "config/tokens.json", "Path to token indexer configuration file")
    flag.Parse()

    // Initialize repository
    repo, err := repository.NewRepository(context.Background())
    if err != nil {
        log.Fatalf("Failed to create repository: %v", err)
    }

    // Create token indexer service
    service, err := tokenindexer.NewService(*configPath, repo)
    if err != nil {
        log.Fatalf("Failed to create token indexer service: %v", err)
    }

    // Setup signal handling
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        sig := <-sigCh
        log.Printf("Received signal: %v", sig)
        cancel()
    }()

    // Start processing messages
    log.Printf("Starting token indexer...")
    
    // Use existing message subscription mechanism
    // This will be handled by the core indexer infrastructure
    // The token indexer service will filter and process only relevant messages

    <-ctx.Done()
    log.Printf("Shutting down...")
}