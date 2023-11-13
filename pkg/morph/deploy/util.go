package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/neorpc"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/invoker"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neofs-contract/common"
	"go.uber.org/zap"
)

func isErrContractAlreadyUpdated(err error) bool {
	return strings.Contains(err.Error(), common.ErrAlreadyUpdated)
}

func isErrTLDNotFound(err error) bool {
	return strings.Contains(err.Error(), "TLD not found")
}

func setGroupInManifest(manif *manifest.Manifest, nefFile nef.File, groupPrivKey *keys.PrivateKey, deployerAcc util.Uint160) {
	contractAddress := state.CreateContractHash(deployerAcc, nefFile.Checksum, manif.Name)
	sig := groupPrivKey.Sign(contractAddress.BytesBE())
	groupPubKey := groupPrivKey.PublicKey()

	ind := -1

	for i := range manif.Groups {
		if manif.Groups[i].PublicKey.Equal(groupPubKey) {
			ind = i
			break
		}
	}

	if ind >= 0 {
		manif.Groups[ind].Signature = sig
		return
	}

	manif.Groups = append(manif.Groups, manifest.Group{
		PublicKey: groupPubKey,
		Signature: sig,
	})
}

// blockchainMonitor is a thin utility around Blockchain providing state
// monitoring.
type blockchainMonitor struct {
	logger *zap.Logger

	blockchain Blockchain

	blockInterval time.Duration

	subID  string
	height atomic.Uint32
}

// newBlockchainMonitor constructs and runs monitor for the given Blockchain.
// Resulting blockchainMonitor must be stopped when no longer needed.
func newBlockchainMonitor(l *zap.Logger, b Blockchain, chNewBlock chan<- struct{}) (*blockchainMonitor, error) {
	ver, err := b.GetVersion()
	if err != nil {
		return nil, fmt.Errorf("request Neo protocol configuration: %w", err)
	}

	initialBlock, err := b.GetBlockCount()
	if err != nil {
		return nil, fmt.Errorf("get current blockchain height: %w", err)
	}

	blockCh := make(chan *block.Block)

	newBlockSubID, err := b.ReceiveBlocks(nil, blockCh)
	if err != nil {
		return nil, fmt.Errorf("subscribe to new blocks of the chain: %w", err)
	}

	res := &blockchainMonitor{
		logger:        l,
		blockchain:    b,
		blockInterval: time.Duration(ver.Protocol.MillisecondsPerBlock) * time.Millisecond,
		subID:         newBlockSubID,
	}

	res.height.Store(initialBlock)

	go func() {
		l.Info("listening to new blocks...")
		for {
			b, ok := <-blockCh
			if !ok {
				close(chNewBlock)
				l.Info("listening to new blocks stopped")
				return
			}

			res.height.Store(b.Index)

			select {
			case chNewBlock <- struct{}{}:
			default:
			}

			l.Info("new block arrived", zap.Uint32("height", b.Index))
		}
	}()

	return res, nil
}

// currentHeight returns current blockchain height.
func (x *blockchainMonitor) currentHeight() uint32 {
	return x.height.Load()
}

// waitForNextBlock blocks until blockchainMonitor encounters new block on the
// chain or provided context is done.
func (x *blockchainMonitor) waitForNextBlock(ctx context.Context) {
	initialBlock := x.currentHeight()

	ticker := time.NewTicker(x.blockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if x.height.Load() > initialBlock {
				return
			}
		}
	}
}

// stop stops running blockchainMonitor. Stopped blockchainMonitor must not be
// used anymore.
func (x *blockchainMonitor) stop() {
	err := x.blockchain.Unsubscribe(x.subID)
	if err != nil {
		x.logger.Warn("failed to cancel subscription to new blocks", zap.Error(err))
	}
}

// readNNSOnChainState reads state of the NeoFS NNS contract in the given
// Blockchain. Returns both nil if contract is missing.
func readNNSOnChainState(b Blockchain) (*state.Contract, error) {
	// NNS must always have ID=1 in the NeoFS Sidechain
	const nnsContractID = 1
	res, err := b.GetContractStateByID(nnsContractID)
	if err != nil {
		if errors.Is(err, neorpc.ErrUnknownContract) {
			return nil, nil
		}
		return nil, fmt.Errorf("read contract state by ID=%d: %w", nnsContractID, err)
	}
	return res, nil
}

type transactionGroupWaiter interface {
	WaitAny(ctx context.Context, vub uint32, hashes ...util.Uint256) (*state.AppExecResult, error)
}

type transactionGroupMonitor struct {
	waiter  transactionGroupWaiter
	pending atomic.Bool
}

func newTransactionGroupMonitor(w transactionGroupWaiter) *transactionGroupMonitor {
	return &transactionGroupMonitor{
		waiter: w,
	}
}

func (x *transactionGroupMonitor) reset() {
	x.pending.Store(false)
}

func (x *transactionGroupMonitor) isPending() bool {
	return x.pending.Load()
}

func (x *transactionGroupMonitor) trackPendingTransactionsAsync(ctx context.Context, vub uint32, txs ...util.Uint256) {
	if len(txs) == 0 {
		panic("missing transactions")
	}

	x.pending.Store(true)

	waitCtx, cancel := context.WithCancel(ctx)

	go func() {
		_, _ = x.waiter.WaitAny(waitCtx, vub, txs...)
		x.reset()
		cancel()
	}()
}

var errInvalidContractDomainRecord = errors.New("invalid contract domain record")

// readContractOnChainStateByDomainName reads address state of contract deployed
// in the given Blockchain and recorded in the NNS with the specified domain
// name. Returns errMissingDomain if domain doesn't exist. Returns
// errMissingDomainRecord if domain has no records. Returns
// errInvalidContractDomainRecord if domain record has invalid/unsupported
// format. Returns [neorpc.ErrUnknownContract] if contract is recorded in the NNS but
// missing in the Blockchain.
func readContractOnChainStateByDomainName(b Blockchain, nnsContract util.Uint160, domainName string) (*state.Contract, error) {
	rec, err := lookupNNSDomainRecord(invoker.New(b, nil), nnsContract, domainName)
	if err != nil {
		return nil, err
	}

	// historically two formats may occur
	addr, err := util.Uint160DecodeStringLE(rec)
	if err != nil {
		addr, err = address.StringToUint160(rec)
		if err != nil {
			return nil, fmt.Errorf("%w: domain record '%s' neither NEO address nor little-endian hex-encoded script hash", errInvalidContractDomainRecord, rec)
		}
	}

	res, err := b.GetContractStateByHash(addr)
	if err != nil {
		return nil, fmt.Errorf("get contract by address=%s: %w", addr, err)
	}

	return res, nil
}
