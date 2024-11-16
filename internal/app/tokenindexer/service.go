package tokenindexer

import (
    "context"
    "encoding/json"
    "os"
    "sync"

    "github.com/stepandra/anton/internal/core"
    "github.com/stepandra/anton/internal/core/filter"
)

// TokenConfig represents configuration for a specific token to track
type TokenConfig struct {
    Address     string `json:"address"`
    Symbol      string `json:"symbol"`
    Decimals    int    `json:"decimals"`
    Description string `json:"description"`
}

// IndexSettings represents settings for the token indexer
type IndexSettings struct {
    SkipSmallTransfers  bool  `json:"skip_small_transfers"`
    MinTransferAmount   int64 `json:"min_transfer_amount"`
    TrackBurns         bool  `json:"track_burns"`
    TrackMints         bool  `json:"track_mints"`
}

// Config represents the token indexer configuration
type Config struct {
    Tokens        []TokenConfig `json:"tokens"`
    TrackTraders  bool         `json:"track_traders"`
    MinTradeAmount int64       `json:"min_trade_amount"`
    IndexSettings IndexSettings `json:"index_settings"`
}

// Service represents the token indexer service
type Service struct {
    config     Config
    repository core.Repository
    mu         sync.RWMutex
    traders    map[string]struct{} // Set of trader addresses
}

// NewService creates a new token indexer service
func NewService(configPath string, repository core.Repository) (*Service, error) {
    // Read config file
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, err
    }

    var config Config
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, err
    }

    return &Service{
        config:     config,
        repository: repository,
        traders:    make(map[string]struct{}),
    }, nil
}

// ProcessMessage processes a message and tracks token transfers and traders
func (s *Service) ProcessMessage(ctx context.Context, msg *core.Message) error {
    // Skip if not a token transfer
    if msg.OperationName != "jetton_transfer" && 
       msg.OperationName != "jetton_internal_transfer" {
        return nil
    }

    // Check if this token is in our tracking list
    isTrackedToken := false
    for _, token := range s.config.Tokens {
        if msg.MinterAddress != nil && msg.MinterAddress.String() == token.Address {
            isTrackedToken = true
            break
        }
    }

    if !isTrackedToken {
        return nil
    }

    // Apply minimum amount filter if configured
    if s.config.IndexSettings.SkipSmallTransfers {
        amount, ok := msg.Payload.Data["amount"].(int64)
        if !ok || amount < s.config.IndexSettings.MinTransferAmount {
            return nil
        }
    }

    // Track traders if enabled
    if s.config.TrackTraders {
        s.mu.Lock()
        if msg.SrcAddress != nil {
            s.traders[msg.SrcAddress.String()] = struct{}{}
        }
        if msg.DstAddress != nil {
            s.traders[msg.DstAddress.String()] = struct{}{}
        }
        s.mu.Unlock()
    }

    return nil
}

// GetTraders returns the list of tracked trader addresses
func (s *Service) GetTraders() []string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    traders := make([]string, 0, len(s.traders))
    for addr := range s.traders {
        traders = append(traders, addr)
    }
    return traders
}

// GetTokenTransfers returns transfers for a specific token
func (s *Service) GetTokenTransfers(ctx context.Context, tokenAddr string) ([]*core.Message, error) {
    // Create filter for token transfers
    f := &filter.MessageFilter{
        MinterAddress:  tokenAddr,
        OperationName: "jetton_transfer",
    }

    return s.repository.GetMessages(ctx, f)
}

// GetTokenHolders returns current token holders and their balances
func (s *Service) GetTokenHolders(ctx context.Context, tokenAddr string) ([]*core.Account, error) {
    // Create filter for token wallets
    f := &filter.AccountFilter{
        MinterAddress: tokenAddr,
        Interface:    "jetton_wallet",
        Latest:      true,
    }

    return s.repository.GetAccounts(ctx, f)
}