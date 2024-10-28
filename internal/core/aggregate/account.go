package aggregate

import (
	"github.com/stepandra/anton/abi"
	"github.com/stepandra/anton/addr"
	"github.com/stepandra/anton/internal/core"
)

type AccountsReq struct {
	Address        *addr.Address    `json:"address"`
	MinterAddress  *addr.Address    `json:"minter_address"`
	Limit          uint64           `json:"limit"`
	Contract       abi.ContractName `json:"contract"`
	SkipNotUpdated bool             `json:"skip_not_updated"`
}

type AccountsRes struct {
	TransactionsCount   int                `json:"transactions_count"`
	OwnedNFTItems       int                `json:"owned_nft_items"`
	OwnedNFTCollections int                `json:"owned_nft_collections"`
	OwnedJettonWallets  int                `json:"owned_jetton_wallets"`
	Items               int                `json:"items"`
	OwnersCount         int                `json:"owners_count"`
	UniqueOwners        []*core.UniqueOwner `json:"unique_owners"`
	OwnedItems          []*core.OwnedItem   `json:"owned_items"`
	Wallets             int                `json:"wallets"`
	TotalSupply         *string             `json:"total_supply,omitempty"`
	OwnedBalance        []*core.OwnedItem   `json:"owned_balance"`
}

type UniqueOwner struct {
	ItemAddress addr.Address `json:"item_address" swaggertype:"string"`
	OwnersCount int         `json:"owners_count"`
}

type OwnedItem struct {
	OwnerAddress addr.Address `json:"owner_address"`
	ItemsCount   int         `json:"items_count"`
	Balance      *string      `json:"balance,omitempty"`
}
