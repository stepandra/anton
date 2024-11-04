package known

import "github.com/stepandra/anton/abi"

var (
    JettonMinter abi.ContractName = "jetton_minter"
    JettonWallet abi.ContractName = "jetton_wallet"

    DedustV2Pool    abi.ContractName = "dedust_v2_pool"
    DedustV2Factory abi.ContractName = "dedust_v2_factory"
    StonFiPool      abi.ContractName = "stonfi_pool"
    StonFiRouter    abi.ContractName = "stonfi_router"

    // Add BCL contracts
    BCLMaster  abi.ContractName = "bcl_master"
    BCLJetton  abi.ContractName = "bcl_jetton"
    BCLWallet  abi.ContractName = "bcl_wallet"

    // Add Gaspump contract
    GaspumpJetton abi.ContractName = "gaspump_jetton"
)

var (
	walletInterfacesSet = map[abi.ContractName]struct{}{
		"wallet_v1r1":          {},
		"wallet_v1r2":          {},
		"wallet_v1r3":          {},
		"wallet_v2r1":          {},
		"wallet_v2r2":          {},
		"wallet_v3r1":          {},
		"wallet_v3r2":          {},
		"wallet_v4r1":          {},
		"wallet_v4r2":          {},
		"wallet_lockup":        {},
		"wallet_highload_v1r1": {},
		"wallet_highload_v1r2": {},
		"wallet_highload_v2r1": {},
		"wallet_highload_v2r2": {},
	}
	walletInterfacesList []abi.ContractName
)

func init() {
	for w := range walletInterfacesSet {
		walletInterfacesList = append(walletInterfacesList, w)
	}
}

func GetAllWalletNames() []abi.ContractName {
	return walletInterfacesList
}

func IsOnlyWalletInterfaces(interfaces []abi.ContractName) bool {
	for _, i := range interfaces {
		if _, ok := walletInterfacesSet[i]; !ok {
			return false
		}
	}
	return true
}
