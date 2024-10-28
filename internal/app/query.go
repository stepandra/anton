package app

import (
	"context"

	"github.com/xssnick/tonutils-go/ton"

	"github.com/stepandra/anton/abi"
	"github.com/stepandra/anton/internal/core"
	"github.com/stepandra/anton/internal/core/aggregate"
	"github.com/stepandra/anton/internal/core/aggregate/history"
	"github.com/stepandra/anton/internal/core/filter"
	"github.com/stepandra/anton/internal/core/repository"
)

type QueryConfig struct {
	DB *repository.DB

	API ton.APIClientWrapped
}

type QueryService interface {
	GetStatistics(ctx context.Context) (*aggregate.Statistics, error)

	GetDefinitions(context.Context) (map[abi.TLBType]abi.TLBFieldsDesc, error)
	GetInterfaces(ctx context.Context) ([]*core.ContractInterface, error)
	GetOperations(ctx context.Context) ([]*core.ContractOperation, error)

	filter.BlockRepository

	GetLabelCategories(context.Context) ([]core.LabelCategory, error)

	filter.AccountRepository
	filter.TransactionRepository
	filter.MessageRepository

	aggregate.AccountRepository
	aggregate.MessageRepository

	history.AccountRepository
	history.TransactionRepository
	history.MessageRepository
}
