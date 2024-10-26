package parser

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun/extra/bunbig"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"github.com/tonindexer/anton/abi"
	"github.com/tonindexer/anton/abi/known"
	"github.com/tonindexer/anton/addr"
	"github.com/tonindexer/anton/internal/app"
	"github.com/tonindexer/anton/internal/core"
)

var (
	dedustFactoryAddr *addr.Address
	stonfiRouterAddr  *addr.Address
)

func init() {
	dedustFactoryAddr = addr.MustFromBase64("EQBfBWT7X2BHg9tXAxzhz2aKiNTU1tpt5NsiK0uSDW_YAJ67")
	stonfiRouterAddr = addr.MustFromBase64("EQB3ncyBUTjZUA5EnFKR5_EnOMI9V1tTEAAPaiU71gc4TiUt")
}

func getMethodByName(i *core.ContractInterface, n string) *abi.GetMethodDesc {
	for it := range i.GetMethodsDesc {
		if i.GetMethodsDesc[it].Name == n {
			return &i.GetMethodsDesc[it]
		}
	}
	return nil
}

func (s *Service) emulateGetMethod(ctx context.Context, d *abi.GetMethodDesc, acc *core.AccountState, args []any) (ret abi.GetMethodExecution, err error) {
	var argsStack abi.VmStack

	if len(acc.Code) == 0 || len(acc.Data) == 0 {
		return ret, errors.Wrapf(app.ErrImpossibleParsing, "no account code or data for %s (%d)", acc.Address.Base64(), acc.LastTxLT)
	}

	if len(d.Arguments) != len(args) {
		return ret, errors.New("length of passed and described arguments does not match")
	}
	for it := range args {
		argsStack = append(argsStack, abi.VmValue{
			VmValueDesc: d.Arguments[it],
			Payload:     args[it],
		})
	}

	defer core.Timer(time.Now(), fmt.Sprintf("emulateGetMethod(%s, %s)", acc.Address.Base64(), d.Name))

	codeBase64, dataBase64, librariesBase64 :=
		base64.StdEncoding.EncodeToString(acc.Code),
		base64.StdEncoding.EncodeToString(acc.Data),
		base64.StdEncoding.EncodeToString(acc.Libraries)

	e, err := abi.NewEmulatorBase64(acc.Address.MustToTonutils(), codeBase64, dataBase64, s.bcConfigBase64, librariesBase64)
	if err != nil {
		return ret, errors.Wrap(err, "new emulator")
	}

	retStack, err := e.RunGetMethod(ctx, d.Name, argsStack, d.ReturnValues)

	ret = abi.GetMethodExecution{
		Name: d.Name,
	}
	for i := range argsStack {
		ret.Receives = append(ret.Receives, argsStack[i].Payload)
	}
	for i := range retStack {
		ret.Returns = append(ret.Returns, retStack[i].Payload)
	}
	if err != nil {
		ret.Error = err.Error()

		lvl := log.Warn()
		lvl.Err(err).
			Str("get_method", d.Name).
			Str("address", acc.Address.Base64()).
			Int32("workchain", acc.Workchain).
			Int64("shard", acc.Shard).
			Uint32("block_seq_no", acc.BlockSeqNo).
			Msg("run get method")
	}
	return ret, nil
}

func (s *Service) emulateGetMethodNoArgs(ctx context.Context, i *core.ContractInterface, gmName string, acc *core.AccountState) (ret abi.GetMethodExecution, err error) {
	gm := getMethodByName(i, gmName)
	if gm == nil {
		// we panic as contract interface was defined, but there are no standard get-method
		panic(fmt.Errorf("%s `%s` get-method was not found", i.Name, gmName))
	}
	if len(gm.Arguments) != 0 {
		// we panic as get-method has the wrong description and dev must fix this bug
		panic(fmt.Errorf("%s `%s` get-method has arguments", i.Name, gmName))
	}

	stack, err := s.emulateGetMethod(ctx, gm, acc, nil)
	if err != nil {
		return ret, errors.Wrapf(err, "%s `%s`", i.Name, gmName)
	}

	return stack, nil
}

func removeGetMethodExecution(acc *core.AccountState, contract abi.ContractName, gm string) bool {
	for it := range acc.ExecutedGetMethods[contract] {
		if acc.ExecutedGetMethods[contract][it].Name != gm {
			continue
		}
		executions := acc.ExecutedGetMethods[contract]
		copy(executions[it:], executions[it+1:])
		acc.ExecutedGetMethods[contract] = executions[:len(executions)-1]
		return true
	}
	return false
}

func appendGetMethodExecution(acc *core.AccountState, contract abi.ContractName, exec *abi.GetMethodExecution) {
	// that's needed to clear array from duplicates (bug is already fixed)
	for removeGetMethodExecution(acc, contract, exec.Name) {
	}
	acc.ExecutedGetMethods[contract] = append(acc.ExecutedGetMethods[contract], *exec)
}

func (s *Service) checkJettonMinter(ctx context.Context, ownerAddr *addr.Address, walletAcc *core.AccountState, others func(context.Context, addr.Address) (*core.AccountState, error)) {
	if minterAddr, ok := s.itemsMinterCache.Get(walletAcc.Address); ok && addr.Equal(walletAcc.MinterAddress, &minterAddr) {
		return
	}

	minter, err := others(ctx, *walletAcc.MinterAddress)
	if err != nil {
		log.Error().Str("minter_address", walletAcc.MinterAddress.Base64()).Err(err).Msg("get jetton minter state")
		return
	}

	desc, err := s.ContractRepo.GetMethodDescription(ctx, known.JettonMinter, "get_wallet_address")
	if err != nil {
		panic(fmt.Errorf("get 'get_wallet_address' method description: %w", err))
	}

	args := []any{ownerAddr.MustToTonutils()}

	walletAcc.Fake = true

	exec, err := s.emulateGetMethod(ctx, &desc, minter, args)
	if err != nil {
		log.Error().Err(err).Msgf("execute %s %s get-method", desc.Name, known.JettonMinter)
		return
	}

	exec.Address = &minter.Address

	appendGetMethodExecution(walletAcc, known.JettonMinter, &exec)
	if exec.Error != "" {
		log.Error().Str("exec_error", exec.Error).Msgf("execute %s %s get-method", desc.Name, known.JettonMinter)
		return
	}

	itemAddr := addr.MustFromTonutils(exec.Returns[0].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
	if addr.Equal(itemAddr, &walletAcc.Address) {
		walletAcc.Fake = false
	}

	if !walletAcc.Fake {
		s.itemsMinterCache.Put(walletAcc.Address, minter.Address)
	}
}

func (s *Service) checkDeDustMinter(ctx context.Context, acc *core.AccountState, others func(context.Context, addr.Address) (*core.AccountState, error)) {
	if minterAddr, ok := s.itemsMinterCache.Get(acc.Address); ok && addr.Equal(dedustFactoryAddr, &minterAddr) {
		return
	}

	factory, err := others(ctx, *dedustFactoryAddr)
	if err != nil {
		log.Error().Str("factory_address", dedustFactoryAddr.Base64()).Err(err).Msg("get dedust v2 factory state")
		return
	}

	acc.Fake = true

	if len(acc.ExecutedGetMethods[known.DedustV2Pool]) < 6 ||
		acc.ExecutedGetMethods[known.DedustV2Pool][0].Name != "get_assets" || acc.ExecutedGetMethods[known.DedustV2Pool][5].Name != "is_stable" ||
		acc.ExecutedGetMethods[known.DedustV2Pool][0].Error != "" || acc.ExecutedGetMethods[known.DedustV2Pool][5].Error != "" {
		return
	}

	desc, err := s.ContractRepo.GetMethodDescription(ctx, known.DedustV2Factory, "get_pool_address")
	if err != nil {
		panic(fmt.Errorf("get 'get_pool_address' method description: %w", err))
	}

	asset0 := acc.ExecutedGetMethods[known.DedustV2Pool][0].Returns[0].(*abi.DedustAsset) //nolint:forcetypeassert // that's ok
	asset1 := acc.ExecutedGetMethods[known.DedustV2Pool][0].Returns[1].(*abi.DedustAsset) //nolint:forcetypeassert // that's ok
	isStable := acc.ExecutedGetMethods[known.DedustV2Pool][5].Returns[0].(bool)           //nolint:forcetypeassert // that's ok

	args := []any{isStable, asset0, asset1}

	exec, err := s.emulateGetMethod(ctx, &desc, factory, args)
	if err != nil {
		log.Error().Err(err).Msgf("execute %s %s get-method", desc.Name, known.DedustV2Factory)
		return
	}

	exec.Address = &factory.Address

	appendGetMethodExecution(acc, known.DedustV2Factory, &exec)
	if exec.Error != "" {
		log.Error().Str("exec_error", exec.Error).Msgf("execute %s %s get-method", desc.Name, known.DedustV2Factory)
		return
	}

	itemAddr := addr.MustFromTonutils(exec.Returns[0].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
	if addr.Equal(itemAddr, &acc.Address) {
		acc.Fake = false
	}

	if !acc.Fake {
		s.itemsMinterCache.Put(acc.Address, factory.Address)
	}
}

func (s *Service) checkStonFiMinter(ctx context.Context, acc *core.AccountState, others func(context.Context, addr.Address) (*core.AccountState, error)) {
	if minterAddr, ok := s.itemsMinterCache.Get(acc.Address); ok && addr.Equal(stonfiRouterAddr, &minterAddr) {
		return
	}

	router, err := others(ctx, *stonfiRouterAddr)
	if err != nil {
		log.Error().Str("router_address", stonfiRouterAddr.Base64()).Err(err).Msg("get stonfi router state")
		return
	}

	acc.Fake = true

	if len(acc.ExecutedGetMethods[known.StonFiPool]) < 1 ||
		acc.ExecutedGetMethods[known.StonFiPool][0].Name != "get_pool_data" ||
		acc.ExecutedGetMethods[known.StonFiPool][0].Error != "" {
		return
	}

	desc, err := s.ContractRepo.GetMethodDescription(ctx, known.StonFiRouter, "get_pool_address")
	if err != nil {
		panic(fmt.Errorf("get 'get_pool_address' method description: %w", err))
	}

	asset0 := acc.ExecutedGetMethods[known.StonFiPool][0].Returns[2].(*address.Address) //nolint:forcetypeassert // that's ok
	asset1 := acc.ExecutedGetMethods[known.StonFiPool][0].Returns[3].(*address.Address) //nolint:forcetypeassert // that's ok

	args := []any{asset0, asset1}

	exec, err := s.emulateGetMethod(ctx, &desc, router, args)
	if err != nil {
		log.Error().Err(err).Msgf("execute %s %s get-method", desc.Name, known.StonFiRouter)
		return
	}

	exec.Address = &router.Address

	appendGetMethodExecution(acc, known.StonFiRouter, &exec)
	if exec.Error != "" {
		log.Error().Str("exec_error", exec.Error).Msgf("execute %s %s get-method", desc.Name, known.StonFiRouter)
		return
	}

	itemAddr := addr.MustFromTonutils(exec.Returns[0].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
	if addr.Equal(itemAddr, &acc.Address) {
		acc.Fake = false
	}

	if !acc.Fake {
		s.itemsMinterCache.Put(acc.Address, router.Address)
	}
}

func (s *Service) callGetMethod(
	ctx context.Context,
	acc *core.AccountState,
	i *core.ContractInterface,
	getMethodDesc *abi.GetMethodDesc,
	others func(context.Context, addr.Address) (*core.AccountState, error),
) error {
	exec, err := s.emulateGetMethodNoArgs(ctx, i, getMethodDesc.Name, acc)
	if err != nil {
		return errors.Wrapf(err, "execute get-method")
	}

	appendGetMethodExecution(acc, i.Name, &exec)
	if exec.Error != "" {
		return nil
	}

	switch getMethodDesc.Name {
	case "get_jetton_data":
		// no action needed after NFT removal

	case "get_wallet_data":
		acc.JettonBalance = bunbig.FromMathBig(exec.Returns[0].(*big.Int))            //nolint:forcetypeassert // panic on wrong interface
		acc.OwnerAddress = addr.MustFromTonutils(exec.Returns[1].(*address.Address))  //nolint:forcetypeassert // panic on wrong interface
		acc.MinterAddress = addr.MustFromTonutils(exec.Returns[2].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface

		if acc.MinterAddress == nil || acc.OwnerAddress == nil {
			return nil
		}

		s.checkJettonMinter(ctx, acc.OwnerAddress, acc, others)
	}

	return nil
}

func (s *Service) callPossibleGetMethods(
	ctx context.Context,
	acc *core.AccountState,
	others func(context.Context, addr.Address) (*core.AccountState, error),
	interfaces []*core.ContractInterface,
) {
	defer core.Timer(time.Now(), "callPossibleGetMethods(%s, %v)", acc.Address.Base64(), acc.Types)

	for _, i := range interfaces {
		for it := range i.GetMethodsDesc {
			d := &i.GetMethodsDesc[it]

			if len(d.Arguments) != 0 {
				continue
			}

			if err := s.callGetMethod(ctx, acc, i, d, others); err != nil {
				log.Error().Err(err).Str("contract_name", string(i.Name)).Str("get_method", d.Name).Msg("execute get-method")
			}
		}

		sort.Slice(acc.ExecutedGetMethods[i.Name], func(it, jt int) bool {
			return acc.ExecutedGetMethods[i.Name][it].Name < acc.ExecutedGetMethods[i.Name][jt].Name
		})

		switch i.Name {
		case known.DedustV2Pool:
			s.checkDeDustMinter(ctx, acc, others)

		case known.StonFiPool:
			s.checkStonFiMinter(ctx, acc, others)
		}
	}
}
