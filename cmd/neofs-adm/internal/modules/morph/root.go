package morph

import (
	"fmt"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	alphabetWalletsFlag             = "alphabet-wallets"
	alphabetSizeFlag                = "size"
	endpointFlag                    = "rpc-endpoint"
	storageWalletFlag               = "storage-wallet"
	storageWalletLabelFlag          = "label"
	storageWalletsNumber            = "wallets-number"
	storageGasCLIFlag               = "initial-gas"
	storageGasConfigFlag            = "storage.initial_gas"
	contractsInitFlag               = "contracts"
	maxObjectSizeInitFlag           = "network.max_object_size"
	maxObjectSizeCLIFlag            = "max-object-size"
	epochDurationInitFlag           = "network.epoch_duration"
	epochDurationCLIFlag            = "epoch-duration"
	incomeRateInitFlag              = "network.basic_income_rate"
	incomeRateCLIFlag               = "basic-income-rate"
	auditFeeInitFlag                = "network.fee.audit"
	auditFeeCLIFlag                 = "audit-fee"
	containerFeeInitFlag            = "network.fee.container"
	containerAliasFeeInitFlag       = "network.fee.container_alias"
	containerFeeCLIFlag             = "container-fee"
	containerAliasFeeCLIFlag        = "container-alias-fee"
	candidateFeeInitFlag            = "network.fee.candidate"
	candidateFeeCLIFlag             = "candidate-fee"
	homomorphicHashDisabledInitFlag = "network.homomorphic_hash_disabled"
	maintenanceModeAllowedInitFlag  = "network.maintenance_mode_allowed"
	homomorphicHashDisabledCLIFlag  = "homomorphic-disabled"
	withdrawFeeInitFlag             = "network.fee.withdraw"
	withdrawFeeCLIFlag              = "withdraw-fee"
	containerDumpFlag               = "dump"
	containerContractFlag           = "container-contract"
	containerIDsFlag                = "cid"
	refillGasAmountFlag             = "gas"
	walletAccountFlag               = "account"
	notaryDepositTillFlag           = "till"
	localDumpFlag                   = "local-dump"
	protoConfigPath                 = "protocol"
	walletAddressFlag               = "wallet-address"
	domainFlag                      = "domain"
	neoAddressesFlag                = "neo-addresses"
	publicKeysFlag                  = "public-keys"
	walletFlag                      = "wallet"
)

var (
	// RootCmd is a root command of config section.
	RootCmd = &cobra.Command{
		Use:   "morph",
		Short: "Section for morph network configuration commands",
	}

	generateAlphabetCmd = &cobra.Command{
		Use:   "generate-alphabet",
		Short: "Generate alphabet wallets for consensus nodes of the morph network",
		PreRun: func(cmd *cobra.Command, _ []string) {
			// PreRun fixes https://github.com/spf13/viper/issues/233
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
		},
		RunE: generateAlphabetCreds,
	}

	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize side chain network with smart-contracts and network settings",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
			_ = viper.BindPFlag(epochDurationInitFlag, cmd.Flags().Lookup(epochDurationCLIFlag))
			_ = viper.BindPFlag(maxObjectSizeInitFlag, cmd.Flags().Lookup(maxObjectSizeCLIFlag))
			_ = viper.BindPFlag(incomeRateInitFlag, cmd.Flags().Lookup(incomeRateCLIFlag))
			_ = viper.BindPFlag(homomorphicHashDisabledInitFlag, cmd.Flags().Lookup(homomorphicHashDisabledCLIFlag))
			_ = viper.BindPFlag(auditFeeInitFlag, cmd.Flags().Lookup(auditFeeCLIFlag))
			_ = viper.BindPFlag(candidateFeeInitFlag, cmd.Flags().Lookup(candidateFeeCLIFlag))
			_ = viper.BindPFlag(containerFeeInitFlag, cmd.Flags().Lookup(containerFeeCLIFlag))
			_ = viper.BindPFlag(containerAliasFeeInitFlag, cmd.Flags().Lookup(containerAliasFeeCLIFlag))
			_ = viper.BindPFlag(withdrawFeeInitFlag, cmd.Flags().Lookup(withdrawFeeCLIFlag))
			_ = viper.BindPFlag(protoConfigPath, cmd.Flags().Lookup(protoConfigPath))
			_ = viper.BindPFlag(localDumpFlag, cmd.Flags().Lookup(localDumpFlag))
		},
		RunE: initializeSideChainCmd,
	}

	generateStorageCmd = &cobra.Command{
		Use:   "generate-storage-wallet",
		Short: "Generate storage node wallet for the morph network",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
			_ = viper.BindPFlag(storageGasConfigFlag, cmd.Flags().Lookup(storageGasCLIFlag))
		},
		RunE: generateStorageCreds,
	}

	refillGasCmd = &cobra.Command{
		Use:   "refill-gas",
		Short: "Refill GAS of storage node's wallet in the morph network",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
			_ = viper.BindPFlag(refillGasAmountFlag, cmd.Flags().Lookup(refillGasAmountFlag))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var gasReceiver util.Uint160
			var err error

			// wallet address is not part of the config
			walletAddress, _ := cmd.Flags().GetString(walletAddressFlag)

			if len(walletAddress) != 0 {
				gasReceiver, err = address.StringToUint160(walletAddress)
				if err != nil {
					return fmt.Errorf("invalid wallet address %s: %w", walletAddress, err)
				}
			} else {
				// storage wallet path is not part of the config
				storageWalletPath, _ := cmd.Flags().GetString(storageWalletFlag)
				if storageWalletPath == "" {
					return fmt.Errorf("missing wallet path (use '--%s <out.json>')", storageWalletFlag)
				}

				w, err := wallet.NewWallet(storageWalletPath)
				if err != nil {
					return fmt.Errorf("can't open wallet: %w", err)
				}

				gasReceiver = w.Accounts[0].Contract.ScriptHash()
			}

			gasAmount, err := parseGASAmount(viper.GetString(refillGasAmountFlag))
			if err != nil {
				return err
			}

			return refillGas(cmd, int64(gasAmount), []util.Uint160{gasReceiver})
		},
	}

	forceNewEpoch = &cobra.Command{
		Use:   "force-new-epoch",
		Short: "Create new NeoFS epoch event in the side chain",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: forceNewEpochCmd,
	}

	removeNodes = &cobra.Command{
		Use:   "remove-nodes key1 [key2 [...]]",
		Short: "Remove storage nodes from the netmap",
		Long:  `Move nodes to the Offline state in the candidates list and tick an epoch to update the netmap`,
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: removeNodesCmd,
	}

	setConfig = &cobra.Command{
		Use:                   "set-config key1=val1 [key2=val2 ...]",
		DisableFlagsInUseLine: true,
		Short:                 "Add/update global config value in the NeoFS network",
		Long: `Add/update global config value in the NeoFS network. 
				If key is unknown, it will be added to the config only with --force flag.
Values for unknown keys are added exactly the way they're provided, no conversion is made ("123" will be stored as "313233" hexadecimal).`,
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		Args: cobra.MinimumNArgs(1),
		RunE: setConfigCmd,
	}

	setPolicy = &cobra.Command{
		Use:                   "set-policy [ExecFeeFactor=<n1>] [StoragePrice=<n2>] [FeePerByte=<n3>]",
		DisableFlagsInUseLine: true,
		Short:                 "Set global policy values",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: setPolicyCmd,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"ExecFeeFactor=", "StoragePrice=", "FeePerByte="}, cobra.ShellCompDirectiveNoSpace
		},
	}

	dumpContractHashesCmd = &cobra.Command{
		Use:   "dump-hashes",
		Short: "Dump deployed contract hashes",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: dumpContractHashes,
	}

	dumpNamesCmd = &cobra.Command{
		Use:   "dump-names",
		Short: "Dump known registred NNS names and expirations",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: dumpNames,
	}

	dumpNetworkConfigCmd = &cobra.Command{
		Use:   "dump-config",
		Short: "Dump NeoFS network config",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: dumpNetworkConfig,
	}

	dumpBalancesCmd = &cobra.Command{
		Use:   "dump-balances",
		Short: "Dump GAS balances",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: dumpBalances,
	}

	updateContractsCmd = &cobra.Command{
		Use:   "update-contracts",
		Short: "Update NeoFS contracts",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: updateContracts,
	}

	dumpContainersCmd = &cobra.Command{
		Use:   "dump-containers",
		Short: "Dump NeoFS containers to file",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: dumpContainers,
	}

	renewDomainCmd = &cobra.Command{
		Use:   "renew-domain",
		Short: "Renew NNS domain",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: renewDomain,
	}

	restoreContainersCmd = &cobra.Command{
		Use:   "restore-containers",
		Short: "Restore NeoFS containers from file",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(alphabetWalletsFlag, cmd.Flags().Lookup(alphabetWalletsFlag))
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: restoreContainers,
	}

	listContainersCmd = &cobra.Command{
		Use:   "list-containers",
		Short: "List NeoFS containers",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: listContainers,
	}

	depositNotaryCmd = &cobra.Command{
		Use:   "deposit-notary",
		Short: "Deposit GAS for notary service",
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: depositNotary,
	}
	netmapCandidatesCmd = &cobra.Command{
		Use:   "netmap-candidates",
		Short: "List netmap candidates nodes",
		Args:  cobra.NoArgs,
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
		},
		RunE: listNetmapCandidatesNodes,
	}

	verifiedNodesDomainCmd = &cobra.Command{
		Use:   "verified-nodes-domain",
		Short: "Group of commands to work with verified domains for the storage nodes",
		Args:  cobra.NoArgs,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(endpointFlag, cmd.Flags().Lookup(endpointFlag))
			_ = viper.BindPFlag(domainFlag, cmd.Flags().Lookup(domainFlag))
		},
	}

	verifiedNodesDomainAccessListCmd = &cobra.Command{
		Use:   "access-list",
		Short: "Get access list for the verified nodes' domain",
		Long:  "List Neo addresses of the storage nodes that have access to use the specified verified domain.",
		Args:  cobra.NoArgs,
		RunE:  verifiedNodesDomainAccessList,
	}

	verifiedNodesDomainSetAccessListCmd = &cobra.Command{
		Use:   "set-access-list",
		Short: "Set access list for the verified nodes' domain",
		Long: "Set list of the storage nodes that have access to use the specified verified domain. " +
			"The list may be either Neo addresses or HEX-encoded public keys of the nodes.",
		Args: cobra.NoArgs,
		PreRun: func(cmd *cobra.Command, _ []string) {
			_ = viper.BindPFlag(walletFlag, cmd.Flags().Lookup(walletFlag))
			_ = viper.BindPFlag(walletAccountFlag, cmd.Flags().Lookup(walletAccountFlag))
			_ = viper.BindPFlag(publicKeysFlag, cmd.Flags().Lookup(publicKeysFlag))
			_ = viper.BindPFlag(neoAddressesFlag, cmd.Flags().Lookup(neoAddressesFlag))
		},
		RunE: verifiedNodesDomainSetAccessList,
	}
)

func init() {
	RootCmd.AddCommand(generateAlphabetCmd)
	generateAlphabetCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	generateAlphabetCmd.Flags().Uint(alphabetSizeFlag, 7, "Amount of alphabet wallets to generate")

	RootCmd.AddCommand(initCmd)
	initCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	initCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	initCmd.Flags().String(contractsInitFlag, "", "Path to archive with compiled NeoFS contracts (default fetched from latest github release)")
	initCmd.Flags().Uint(epochDurationCLIFlag, 240, "Amount of side chain blocks in one NeoFS epoch")
	initCmd.Flags().Uint(maxObjectSizeCLIFlag, 67108864, "Max single object size in bytes")
	initCmd.Flags().Bool(homomorphicHashDisabledCLIFlag, false, "Disable object homomorphic hashing")
	// Defaults are taken from neo-preodolenie.
	initCmd.Flags().Uint64(containerFeeCLIFlag, 1000, "Container registration fee")
	initCmd.Flags().Uint64(containerAliasFeeCLIFlag, 500, "Container alias fee")
	initCmd.Flags().String(protoConfigPath, "", "Path to the consensus node configuration")
	initCmd.Flags().String(localDumpFlag, "", "Path to the blocks dump file")

	RootCmd.AddCommand(deployCmd)

	RootCmd.AddCommand(generateStorageCmd)
	generateStorageCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	generateStorageCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	generateStorageCmd.Flags().String(storageWalletFlag, "", "Path to new storage node wallet(s)")
	generateStorageCmd.Flags().String(storageGasCLIFlag, "", "Initial amount of GAS to transfer")
	generateStorageCmd.Flags().Uint32(storageWalletsNumber, 1, "Number of wallets to generate, if more than 1, suffix-number will be added to the filename")
	generateStorageCmd.Flags().StringP(storageWalletLabelFlag, "l", "", "Wallet label")

	RootCmd.AddCommand(forceNewEpoch)
	forceNewEpoch.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	forceNewEpoch.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")

	RootCmd.AddCommand(removeNodes)
	removeNodes.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	removeNodes.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")

	RootCmd.AddCommand(setPolicy)
	setPolicy.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	setPolicy.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")

	RootCmd.AddCommand(dumpContractHashesCmd)
	dumpContractHashesCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	dumpContractHashesCmd.Flags().String(customZoneFlag, "", "Custom zone to search.")

	RootCmd.AddCommand(dumpNamesCmd)
	dumpNamesCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	dumpNamesCmd.Flags().StringP(nameDomainFlag, "d", "", "Filter by domain")

	RootCmd.AddCommand(dumpNetworkConfigCmd)
	dumpNetworkConfigCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")

	RootCmd.AddCommand(setConfig)
	setConfig.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	setConfig.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	setConfig.Flags().Bool(forceConfigSet, false, "Force setting not well-known configuration key")

	RootCmd.AddCommand(dumpBalancesCmd)
	dumpBalancesCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	dumpBalancesCmd.Flags().BoolP(dumpBalancesStorageFlag, "s", false, "Dump balances of storage nodes from the current netmap")
	dumpBalancesCmd.Flags().BoolP(dumpBalancesAlphabetFlag, "a", false, "Dump balances of alphabet contracts")
	dumpBalancesCmd.Flags().BoolP(dumpBalancesProxyFlag, "p", false, "Dump balances of the proxy contract")
	dumpBalancesCmd.Flags().Bool(dumpBalancesUseScriptHashFlag, false, "Use script-hash format for addresses")

	RootCmd.AddCommand(updateContractsCmd)
	updateContractsCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	updateContractsCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	updateContractsCmd.Flags().String(contractsInitFlag, "", "Path to archive with compiled NeoFS contracts (default fetched from latest github release)")

	RootCmd.AddCommand(dumpContainersCmd)
	dumpContainersCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	dumpContainersCmd.Flags().String(containerDumpFlag, "", "File where to save dumped containers")
	dumpContainersCmd.Flags().String(containerContractFlag, "", "Container contract hash (for networks without NNS)")
	dumpContainersCmd.Flags().StringSlice(containerIDsFlag, nil, "Containers to dump")

	RootCmd.AddCommand(renewDomainCmd)
	renewDomainCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	renewDomainCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	renewDomainCmd.Flags().StringP(nameDomainFlag, "d", "", "Domain")
	renewDomainCmd.Flags().BoolP(recursiveFlag, "u", false, "Recursive (renew all subdomain as well)")

	RootCmd.AddCommand(restoreContainersCmd)
	restoreContainersCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	restoreContainersCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	restoreContainersCmd.Flags().String(containerDumpFlag, "", "File to restore containers from")
	restoreContainersCmd.Flags().StringSlice(containerIDsFlag, nil, "Containers to restore")

	RootCmd.AddCommand(listContainersCmd)
	listContainersCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	listContainersCmd.Flags().String(containerContractFlag, "", "Container contract hash (for networks without NNS)")

	RootCmd.AddCommand(refillGasCmd)
	refillGasCmd.Flags().String(alphabetWalletsFlag, "", "Path to alphabet wallets dir")
	refillGasCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	refillGasCmd.Flags().String(storageWalletFlag, "", "Path to storage node wallet")
	refillGasCmd.Flags().String(walletAddressFlag, "", "Address of wallet")
	refillGasCmd.Flags().String(refillGasAmountFlag, "", "Additional amount of GAS to transfer")
	refillGasCmd.MarkFlagsMutuallyExclusive(walletAddressFlag, storageWalletFlag)

	RootCmd.AddCommand(depositNotaryCmd)
	depositNotaryCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")
	depositNotaryCmd.Flags().String(storageWalletFlag, "", "Path to storage node wallet")
	depositNotaryCmd.Flags().String(walletAccountFlag, "", "Wallet account address")
	depositNotaryCmd.Flags().String(refillGasAmountFlag, "", "Amount of GAS to deposit")
	depositNotaryCmd.Flags().String(notaryDepositTillFlag, "", "Notary deposit duration in blocks")

	RootCmd.AddCommand(netmapCandidatesCmd)
	netmapCandidatesCmd.Flags().StringP(endpointFlag, "r", "", "N3 RPC node endpoint")

	cmd := verifiedNodesDomainAccessListCmd
	fs := cmd.Flags()
	fs.StringP(endpointFlag, "r", "", "NeoFS Sidechain RPC endpoint")
	_ = cmd.MarkFlagRequired(endpointFlag)
	fs.StringP(domainFlag, "d", "", "Verified domain of the storage nodes. Must be a valid NeoFS NNS domain (e.g. 'nodes.some-org.neofs')")
	_ = cmd.MarkFlagRequired(domainFlag)

	verifiedNodesDomainCmd.AddCommand(cmd)

	cmd = verifiedNodesDomainSetAccessListCmd
	fs = cmd.Flags()
	fs.StringP(walletFlag, "w", "", "Path to the Neo wallet file")
	_ = cmd.MarkFlagRequired(walletFlag)
	fs.StringP(walletAccountFlag, "a", "", "Optional Neo address of the wallet account for signing transactions. "+
		"If omitted, default change address from the wallet is used")
	fs.StringP(endpointFlag, "r", "", "NeoFS Sidechain RPC endpoint")
	_ = cmd.MarkFlagRequired(endpointFlag)
	fs.StringP(domainFlag, "d", "", "Verified domain of the storage nodes. Must be a valid NeoFS NNS domain (e.g. 'nodes.some-org.neofs')")
	_ = cmd.MarkFlagRequired(domainFlag)
	fs.StringSlice(neoAddressesFlag, nil, "Neo addresses resolved from public keys of the storage nodes")
	fs.StringSlice(publicKeysFlag, nil, "HEX-encoded public keys of the storage nodes")
	cmd.MarkFlagsMutuallyExclusive(publicKeysFlag, neoAddressesFlag)

	verifiedNodesDomainCmd.AddCommand(cmd)

	RootCmd.AddCommand(verifiedNodesDomainCmd)
}
