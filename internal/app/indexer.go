package app

import (
	"github.com/xssnick/tonutils-go/ton"
	"github.com/stepandra/anton/addr"
	"github.com/stepandra/anton/internal/core/repository"
)

type IndexerConfig struct {
	DB *repository.DB

	API ton.APIClientWrapped

	Fetcher FetcherService
	Parser  ParserService

	FromBlock uint32
	Workers   int
	
	// TargetAccounts is a list of accounts to index. If empty, all accounts will be indexed
	TargetAccounts []addr.Address
}

type IndexerService interface {
	Start() error
	Stop()
}