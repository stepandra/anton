package core

import (
	"github.com/stepandra/anton/addr"
)

// UniqueOwner represents an item with multiple owners
type UniqueOwner struct {
	ItemAddress addr.Address `json:"item_address" swaggertype:"string"`
	OwnersCount int         `json:"owners_count"`
}

// OwnedItem represents ownership details
type OwnedItem struct {
	OwnerAddress addr.Address `json:"owner_address" swaggertype:"string"`
	ItemsCount   int         `json:"items_count"`
	Balance      *string     `json:"balance,omitempty"`
}
