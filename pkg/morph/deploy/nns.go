package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/invoker"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/nns"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/unwrap"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"go.uber.org/zap"
)

// various NNS domain names.
const (
	domainBootstrap             = "bootstrap"
	domainDesignateNotaryPrefix = "designate-committee-notary-"
	domainDesignateNotaryTx     = domainDesignateNotaryPrefix + "tx." + domainBootstrap
	domainContractAddresses     = "neofs"
)

func designateNotarySignatureDomainForMember(memberIndex int) string {
	return fmt.Sprintf("%s%d.%s", domainDesignateNotaryPrefix, memberIndex, domainBootstrap)
}

func committeeGroupDomainForMember(memberIndex int) string {
	return fmt.Sprintf("committee-group-%d.%s", memberIndex, domainBootstrap)
}

// various methods of the NeoFS NNS contract.
const (
	methodNNSRegister  = "register"
	methodNNSResolve   = "resolve"
	methodNNSAddRecord = "addRecord"
	methodNNSSetRecord = "setRecord"
)

// default NNS domain settings. See DNS specification and also
// https://www.ripe.net/publications/docs/ripe-203.
const (
	nnsRefresh = 3600
	nnsRetry   = 600
	nnsExpire  = int64(10 * 365 * 24 * time.Hour / time.Second)
	nnsMinimum = 3600
)

// various NNS errors.
var (
	errMissingDomain       = errors.New("missing domain")
	errMissingDomainRecord = errors.New("missing domain record")
)

// deployNNSContractPrm groups parameters of NeoFS NNS contract deployment.
type deployNNSContractPrm struct {
	logger *zap.Logger

	blockchain Blockchain

	// based on blockchain
	monitor *blockchainMonitor

	localAcc *wallet.Account

	localNEF      nef.File
	localManifest manifest.Manifest
	systemEmail   string

	// optional constructor of private key for the committee group. If set, it is
	// used only when contract is missing.
	initCommitteeGroupKey func() (*keys.PrivateKey, error)
}

// initNNSContract synchronizes NNS contract with the chain and returns the
// address. Success is the presence of NNS contract in the chain with ID=1.
// initNNSContract returns any error encountered due to which the contract
// cannot be synchronized in any way. For example, problems that can be fixed on
// the chain in the background (manually or automatically over time) do not stop
// the procedure. Breaking the context stops execution immediately (so hangup is
// not possible) and the function returns an error. In this case,
// initNNSContract can be re-called (expected after application restart): all
// previously succeeded actions will be skipped, and execution will be continued
// from the last failed stage.
//
// If contract is missing and deployNNSContractPrm.initCommitteeGroupKey is provided,
// initNNSContract attempts to deploy local contract.
func initNNSContract(ctx context.Context, prm deployNNSContractPrm) (res util.Uint160, err error) {
	var managementContract *management.Contract
	var sentTxValidUntilBlock uint32
	var committeeGroupKey *keys.PrivateKey

	for ; ; prm.monitor.waitForNextBlock(ctx) {
		select {
		case <-ctx.Done():
			return res, fmt.Errorf("wait for NNS contract synchronization: %w", ctx.Err())
		default:
		}

		prm.logger.Info("reading on-chain state of the NNS contract by ID=1")

		stateOnChain, err := readNNSOnChainState(prm.blockchain)
		if err != nil {
			prm.logger.Error("failed to read on-chain state of the NNS contract, will try again later", zap.Error(err))
			continue
		}

		if stateOnChain != nil {
			// declared in https://github.com/nspcc-dev/neofs-contract sources
			const nnsContractName = "NameService"
			if stateOnChain.Manifest.Name != nnsContractName {
				return res, fmt.Errorf("wrong name of the contract with ID=1: expected '%s', got '%s'",
					nnsContractName, stateOnChain.Manifest.Name)
			}

			return stateOnChain.Hash, nil
		}

		if prm.initCommitteeGroupKey == nil {
			prm.logger.Info("NNS contract is missing on the chain but attempts to deploy are disabled, will wait for background deployment")
			continue
		}

		prm.logger.Info("NNS contract is missing on the chain, contract needs to be deployed")

		if committeeGroupKey == nil {
			prm.logger.Info("initializing private key for the committee group...")

			committeeGroupKey, err = prm.initCommitteeGroupKey()
			if err != nil {
				prm.logger.Error("failed to init committee group key, will try again later", zap.Error(err))
				continue
			}

			prm.logger.Info("private key of the committee group has been initialized", zap.Stringer("public key", committeeGroupKey.PublicKey()))
		}

		if sentTxValidUntilBlock > 0 {
			prm.logger.Info("transaction deploying NNS contract was sent earlier, checking relevance...")

			if cur := prm.monitor.currentHeight(); cur <= sentTxValidUntilBlock {
				prm.logger.Info("previously sent transaction deploying NNS contract may still be relevant, will wait for the outcome",
					zap.Uint32("current height", cur), zap.Uint32("retry after height", sentTxValidUntilBlock))
				continue
			}

			prm.logger.Info("previously sent transaction deploying NNS contract expired without side-effect")
		}

		prm.logger.Info("sending new transaction deploying NNS contract...")

		if managementContract == nil {
			localActor, err := actor.NewSimple(prm.blockchain, prm.localAcc)
			if err != nil {
				prm.logger.Warn("NNS contract is missing on the chain but attempts to deploy are disabled, will try again later")
				continue
			}

			managementContract = management.New(localActor)

			setGroupInManifest(&prm.localManifest, prm.localNEF, committeeGroupKey, prm.localAcc.ScriptHash())
		}

		// just to definitely avoid mutation
		nefCp := prm.localNEF
		manifestCp := prm.localManifest

		_, vub, err := managementContract.Deploy(&nefCp, &manifestCp, []interface{}{
			[]interface{}{
				[]interface{}{domainBootstrap, prm.systemEmail},
				[]interface{}{domainContractAddresses, prm.systemEmail},
			},
		})
		if err != nil {
			sentTxValidUntilBlock = 0
			if isErrNotEnoughGAS(err) {
				prm.logger.Info("not enough GAS to deploy NNS contract, will try again later")
			} else {
				prm.logger.Error("failed to send transaction deploying NNS contract, will try again later", zap.Error(err))
			}
			continue
		}

		sentTxValidUntilBlock = vub

		prm.logger.Info("transaction deploying NNS contract has been successfully sent, will wait for the outcome")
	}
}

// lookupNNSDomainRecord looks up for the 1st record of the NNS domain with
// given name. Returns errMissingDomain if domain doesn't exist. Returns
// errMissingDomainRecord if domain has no records.
func lookupNNSDomainRecord(inv *invoker.Invoker, nnsContract util.Uint160, domainName string) (string, error) {
	item, err := unwrap.Item(inv.Call(nnsContract, methodNNSResolve, domainName, int64(nns.TXT)))
	if err != nil {
		if strings.Contains(err.Error(), "token not found") {
			return "", errMissingDomain
		}

		return "", fmt.Errorf("call '%s' method of the NNS contract: %w", methodNNSResolve, err)
	}

	arr, ok := item.Value().([]stackitem.Item)
	if !ok {
		if _, ok = item.(stackitem.Null); !ok {
			return "", fmt.Errorf("malformed/unsupported response of the NNS '%s' method: expected array, got %s",
				methodNNSResolve, item.Type())
		}
	} else if len(arr) > 0 {
		b, err := arr[0].TryBytes()
		if err != nil {
			return "", fmt.Errorf("malformed/unsupported 1st array item of the NNS '%s' method response (expected %v): %w",
				methodNNSResolve, stackitem.ByteArrayT, err)
		}

		return string(b), nil
	}

	return "", errMissingDomainRecord
}

// updateNNSContractPrm groups parameters of NeoFS NNS contract update.
type updateNNSContractPrm struct {
	logger *zap.Logger

	blockchain Blockchain

	// based on blockchain
	monitor *blockchainMonitor

	localAcc *wallet.Account

	localNEF      nef.File
	localManifest manifest.Manifest
	systemEmail   string

	committee         keys.PublicKeys
	committeeGroupKey *keys.PrivateKey

	// constructor of extra arguments to be passed into method updating the
	// contract. If returns both nil, no data is passed (noExtraUpdateArgs may be
	// used).
	buildVersionedExtraUpdateArgs func(versionOnChain contractVersion) ([]interface{}, error)

	onNotaryDepositDeficiency notaryDepositDeficiencyHandler
}

// updateNNSContract synchronizes on-chain NNS contract (its presence is a
// precondition) with the local one represented by compiled executables. If
// on-chain version is greater or equal to the local one, nothing happens.
// Otherwise, transaction calling 'update' method is sent.
//
// Local manifest is extended with committee group represented by the
// parameterized private key.
//
// Function behaves similar to initNNSContract in terms of context.
func updateNNSContract(ctx context.Context, prm updateNNSContractPrm) error {
	bLocalNEF, err := prm.localNEF.Bytes()
	if err != nil {
		// not really expected
		return fmt.Errorf("encode local NEF of the NNS contract into binary: %w", err)
	}

	jLocalManifest, err := json.Marshal(prm.localManifest)
	if err != nil {
		// not really expected
		return fmt.Errorf("encode local manifest of the NNS contract into JSON: %w", err)
	}

	committeeActor, err := newCommitteeNotaryActor(prm.blockchain, prm.localAcc, prm.committee)
	if err != nil {
		return fmt.Errorf("create Notary service client sending transactions to be signed by the committee: %w", err)
	}

	localVersion, err := readContractLocalVersion(prm.blockchain, prm.localNEF, prm.localManifest)
	if err != nil {
		return fmt.Errorf("read version of the local NNS contract: %w", err)
	}

	var updateTxValidUntilBlock uint32

	for ; ; prm.monitor.waitForNextBlock(ctx) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for NNS contract synchronization: %w", ctx.Err())
		default:
		}

		prm.logger.Info("reading on-chain state of the NNS contract...")

		nnsOnChainState, err := readNNSOnChainState(prm.blockchain)
		if err != nil {
			prm.logger.Error("failed to read on-chain state of the NNS contract, will try again later", zap.Error(err))
			continue
		} else if nnsOnChainState == nil {
			// NNS contract must be deployed at this stage
			return errors.New("missing required NNS contract on the chain")
		}

		if nnsOnChainState.NEF.Checksum == prm.localNEF.Checksum {
			// manifests may differ, but currently we should bump internal contract version
			// (i.e. change NEF) to make such updates. Right now they are not supported due
			// to dubious practical need
			// Track https://github.com/nspcc-dev/neofs-contract/issues/340
			prm.logger.Info("same local and on-chain checksums of the NNS contract NEF, update is not needed")
			return nil
		}

		prm.logger.Info("NEF checksums of the on-chain and local NNS contracts differ, need an update")

		versionOnChain, err := readContractOnChainVersion(prm.blockchain, nnsOnChainState.Hash)
		if err != nil {
			prm.logger.Error("failed to read on-chain version of the NNS contract, will try again later", zap.Error(err))
			continue
		}

		if v := localVersion.cmp(versionOnChain); v == -1 {
			prm.logger.Info("local contract version is < than the on-chain one, update is not needed",
				zap.Stringer("local", localVersion), zap.Stringer("on-chain", versionOnChain))
			return nil
		} else if v == 0 {
			return fmt.Errorf("local and on-chain contracts have different NEF checksums but same version '%s'", versionOnChain)
		}

		extraUpdateArgs, err := prm.buildVersionedExtraUpdateArgs(versionOnChain)
		if err != nil {
			prm.logger.Error("failed to prepare build extra arguments for NNS contract update, will try again later",
				zap.Stringer("on-chain version", versionOnChain), zap.Error(err))
			continue
		}

		setGroupInManifest(&prm.localManifest, prm.localNEF, prm.committeeGroupKey, prm.localAcc.ScriptHash())

		var vub uint32

		// we pre-check 'already updated' case via MakeCall in order to not potentially
		// wait for previously sent transaction to be expired (condition below) and
		// immediately succeed
		tx, err := committeeActor.MakeCall(nnsOnChainState.Hash, methodUpdate,
			bLocalNEF, jLocalManifest, extraUpdateArgs)
		if err != nil {
			if isErrContractAlreadyUpdated(err) {
				// note that we can come here only if local version is > than the on-chain one
				// (compared above)
				prm.logger.Info("NNS contract has already been updated, skip")
				return nil
			}
		} else {
			if updateTxValidUntilBlock > 0 {
				prm.logger.Info("transaction updating NNS contract was sent earlier, checking relevance...")

				if cur := prm.monitor.currentHeight(); cur <= updateTxValidUntilBlock {
					prm.logger.Info("previously sent transaction updating NNS contract may still be relevant, will wait for the outcome",
						zap.Uint32("current height", cur), zap.Uint32("retry after height", updateTxValidUntilBlock))
					continue
				}

				prm.logger.Info("previously sent transaction updating NNS contract expired without side-effect")
			}

			prm.logger.Info("sending new transaction updating NNS contract...")

			_, _, vub, err = committeeActor.Notarize(tx, nil)
		}
		if err != nil {
			lackOfGAS := isErrNotEnoughGAS(err)
			// here lackOfGAS=true always means lack of Notary balance and not related to
			// the main transaction itself
			if !lackOfGAS {
				if !isErrNotaryDepositExpires(err) {
					prm.logger.Error("failed to send transaction deploying NNS contract, will try again later", zap.Error(err))
					continue
				}
			}

			// same approach with in-place deposit is going to be used in other functions.
			// Consider replacement with background process (e.g. blockchainMonitor
			// internal) which periodically checks Notary balance and updates it when, for
			// example, balance goes lower than 20% of desired amount or expires soon. With
			// this approach functions like current will not try to make a deposit, but
			// simply wait until it becomes enough.
			prm.onNotaryDepositDeficiency(lackOfGAS)

			continue
		}

		updateTxValidUntilBlock = vub

		prm.logger.Info("transaction updating NNS contract has been successfully sent, will wait for the outcome")
	}
}
