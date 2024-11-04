package parser

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/stepandra/anton/abi"
	"github.com/stepandra/anton/addr"
	"github.com/stepandra/anton/internal/app"
	"github.com/stepandra/anton/internal/core"
)

func matchByAddress(acc *core.AccountState, addresses []*addr.Address) bool {
	for _, a := range addresses {
		if addr.Equal(a, &acc.Address) {
			return true
		}
	}
	return false
}

func matchByGetMethods(acc *core.AccountState, getMethodHashes []int32) bool {
	if len(acc.GetMethodHashes) == 0 || len(getMethodHashes) == 0 {
		return false
	}
	for _, x := range getMethodHashes {
		var found bool
		for _, y := range acc.GetMethodHashes {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func interfaceMatched(acc *core.AccountState, i *core.ContractInterface) bool {
	defer core.Timer(time.Now(), "interfaceMatched(%s, %s)", acc.Address.Base64(), i.Name)

	if matchByAddress(acc, i.Addresses) {
		return true
	}

	if len(acc.Code) != 0 && len(i.Code) != 0 && bytes.Equal(acc.CodeHash, i.CodeHash) {
		return true
	}

	if len(i.Addresses) == 0 && len(i.Code) == 0 && matchByGetMethods(acc, i.GetMethodHashes) {
		// match by get methods only if code and addresses are not set
		return true
	}

	return false
}

func (s *Service) determineInterfaces(ctx context.Context, acc *core.AccountState) ([]*core.ContractInterface, error) {
	var ret []*core.ContractInterface

	defer core.Timer(time.Now(), "determineInterfaces(%s)", acc.Address.Base64())

	interfaces, err := s.ContractRepo.GetInterfaces(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get contract interfaces")
	}

	for _, i := range interfaces {
		if interfaceMatched(acc, i) {
			ret = append(ret, i)
		}
	}

	return ret, nil
}

func (s *Service) ParseAccountData(
	ctx context.Context,
	acc *core.AccountState,
	others func(context.Context, addr.Address) (*core.AccountState, error),
) error {
	if s.ContractRepo == nil {
		return errors.Wrap(app.ErrImpossibleParsing, "no contract repository")
	}

	defer core.Timer(time.Now(), "ParseAccountData(%s)", acc.Address.Base64())

	s.accountParseSemaphore <- struct{}{}
	defer func() { <-s.accountParseSemaphore }()

	interfaces, err := s.determineInterfaces(ctx, acc)
	if err != nil {
		return errors.Wrapf(err, "determine contract interfaces")
	}
	if len(interfaces) == 0 {
		return errors.Wrap(app.ErrImpossibleParsing, "unknown contract interfaces")
	}

	acc.Types = nil
	for _, i := range interfaces {
		acc.Types = append(acc.Types, i.Name)
	}
	acc.ExecutedGetMethods = map[abi.ContractName][]abi.GetMethodExecution{}

	s.CallPossibleGetMethods(ctx, acc, others, interfaces)

	return nil
}

func (s *Service) ParseAccountContractData(
	ctx context.Context,
	contractDesc *core.ContractInterface,
	acc *core.AccountState,
	others func(context.Context, addr.Address) (*core.AccountState, error),
) error {
	if !interfaceMatched(acc, contractDesc) {
		return app.ErrUnmatchedContractInterface
	}

	s.accountParseSemaphore <- struct{}{}
	defer func() { <-s.accountParseSemaphore }()

	var contractTypeSet bool
	for _, t := range acc.Types {
		if t == contractDesc.Name {
			contractTypeSet = true
			break
		}
	}
	if !contractTypeSet {
		acc.Types = append(acc.Types, contractDesc.Name)
	}

	if acc.ExecutedGetMethods == nil {
		acc.ExecutedGetMethods = map[abi.ContractName][]abi.GetMethodExecution{}
	}
	delete(acc.ExecutedGetMethods, contractDesc.Name)

	s.CallPossibleGetMethods(ctx, acc, others, []*core.ContractInterface{contractDesc})

	return nil
}

func (s *Service) ExecuteAccountGetMethod(
	ctx context.Context,
	contract abi.ContractName,
	getMethod string,
	acc *core.AccountState,
	others func(context.Context, addr.Address) (*core.AccountState, error),
) error {
	if s.ContractRepo == nil {
		return errors.Wrap(app.ErrImpossibleParsing, "no contract repository")
	}

	s.accountParseSemaphore <- struct{}{}
	defer func() { <-s.accountParseSemaphore }()

	interfaces, err := s.determineInterfaces(ctx, acc)
	if err != nil {
		return errors.Wrapf(err, "determine contract interfaces")
	}
	if len(interfaces) == 0 {
		return errors.Wrap(app.ErrImpossibleParsing, "unknown contract interfaces")
	}

	var (
		i *core.ContractInterface
		d *abi.GetMethodDesc
	)
	for _, i = range interfaces {
		if i.Name == contract {
			break
		}
	}
	if i == nil {
		return errors.Wrapf(core.ErrInvalidArg,
			"cannot find '%s' interface description for '%s' account", contract, acc.Address)
	}
	for it := range i.GetMethodsDesc {
		if i.GetMethodsDesc[it].Name == getMethod {
			d = &i.GetMethodsDesc[it]
			break
		}
	}
	if d == nil {
		return errors.Wrapf(core.ErrInvalidArg,
			"cannot find '%s' get-method description for '%s' account and '%s' interface", getMethod, acc.Address, contract)
	}

	acc.Types = nil
	for _, i := range interfaces {
		acc.Types = append(acc.Types, i.Name)
	}
	if acc.ExecutedGetMethods == nil {
		acc.ExecutedGetMethods = map[abi.ContractName][]abi.GetMethodExecution{}
	}

	return s.callGetMethod(ctx, acc, i, d, others)
}
