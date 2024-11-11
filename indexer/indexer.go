package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/lightlink-network/ll-bridge-api/contracts/L2CrossDomainMessenger"
	"github.com/lightlink-network/ll-bridge-api/contracts/L2ToL1MessagePasser"
	"github.com/lightlink-network/ll-bridge-api/database"
	"github.com/lightlink-network/ll-bridge-api/ethereum"
	"github.com/lightlink-network/ll-bridge-api/lightlink"
)

var (
	SentMessageEventABI     = "SentMessage(address,address,bytes,uint256,uint256)"
	SentMessageEventABIHash = crypto.Keccak256Hash([]byte(SentMessageEventABI))

	SentMessageExtension1ABI     = "SentMessageExtension1(address,uint256)"
	SentMessageExtension1ABIHash = crypto.Keccak256Hash([]byte(SentMessageExtension1ABI))

	MessagePassedEventABI     = "MessagePassed(uint256,address,address,uint256,uint256,bytes,bytes32)"
	MessagePassedEventABIHash = crypto.Keccak256Hash([]byte(MessagePassedEventABI))
)

type Indexer struct {
	lightlink *lightlink.Client
	ethereum  *ethereum.Client
	database  *database.Database
	logger    *slog.Logger
}

type IndexerOpts struct {
	Lightlink *lightlink.ClientOpts
	Ethereum  *ethereum.ClientOpts
	Database  *database.DatabaseOpts
	Logger    *slog.Logger
}

func NewIndexer(opts IndexerOpts) (*Indexer, error) {
	lightlink, err := lightlink.NewClient(*opts.Lightlink)
	if err != nil {
		return nil, fmt.Errorf("failed to create lightlink client: %w", err)
	}

	ethereum, err := ethereum.NewClient(*opts.Ethereum)
	if err != nil {
		return nil, fmt.Errorf("failed to create ethereum client: %w", err)
	}

	database, err := database.NewDatabase(*opts.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	if err := database.CreateIndexes(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to create database indexes: %w", err)
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Indexer{
		lightlink: lightlink,
		ethereum:  ethereum,
		database:  database,
		logger:    opts.Logger,
	}, nil
}

func (i *Indexer) Run(ctx context.Context) error {
	errChan := make(chan error, 2)
	doneChan := make(chan struct{}, 2)

	go func() {
		err := i.indexLightLink(ctx)
		errChan <- err
		doneChan <- struct{}{}
	}()

	go func() {
		err := i.indexEthereum(ctx)
		errChan <- err
		doneChan <- struct{}{}
	}()

	// Wait for both indexers to finish
	var firstErr error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errChan:
			if err != nil && firstErr == nil {
				firstErr = err
			}
		case <-doneChan:
			continue
		}
	}

	return firstErr
}

type CrossChainMessageV1 struct {
	Target       common.Address
	Sender       common.Address
	Message      []byte
	MessageNonce *big.Int
	GasLimit     *big.Int
	Value        *big.Int `json:"value,omitempty"`
}

func (i *Indexer) txToCrossChainMessageV1(txHash common.Hash) (*CrossChainMessageV1, *uint64, *uint64, error) {
	var receipt *types.Receipt
	var err error

	receipt, err = i.ethereum.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		receipt, err = i.lightlink.TransactionReceipt(context.Background(), txHash)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get transaction receipt on ethereum or lightlink: %w", err)
		}
	}

	// parse data
	contractAbi, err := abi.JSON(strings.NewReader(string(L2CrossDomainMessenger.L2CrossDomainMessengerABI)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse contract abi: %w", err)
	}

	var message CrossChainMessageV1

	// filter logs for log.address === messenger.address
	for _, log := range receipt.Logs {
		if log.Address == i.ethereum.Opts.L1CrossDomainMessengerAddress || log.Address == i.lightlink.Opts.L2CrossDomainMessengerAddress {
			// get cross chain message for deposit so we can calculate message hash
			// we store the message hash in the db so we can check if it's been relayed on L2
			if log.Topics[0] == SentMessageEventABIHash {
				message.Target = common.BytesToAddress(log.Topics[1].Bytes())

				err := contractAbi.UnpackIntoInterface(&message, "SentMessage", log.Data)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to unpack SentMessage: %w", err)
				}
			}

			// get the ETH value of the deposit tx from the extension event
			if log.Topics[0] == SentMessageExtension1ABIHash {
				err := contractAbi.UnpackIntoInterface(&message, "SentMessageExtension1", log.Data)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to unpack SentMessageExtension1: %w", err)
				}
			}
		}
	}

	gasPrice := uint64(0)
	if receipt.EffectiveGasPrice != nil {
		gasPrice = receipt.EffectiveGasPrice.Uint64()
	}

	return &message, &receipt.GasUsed, &gasPrice, nil
}

func (i *Indexer) getWithdrawalHash(txHash common.Hash) (*common.Hash, error) {
	receipt, err := i.lightlink.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt on lightlink: %w", err)
	}

	// parse data
	contractAbi, err := abi.JSON(strings.NewReader(string(L2ToL1MessagePasser.L2ToL1MessagePasserABI)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract abi: %w", err)
	}

	var message L2ToL1MessagePasser.L2ToL1MessagePasserMessagePassed

	// filter logs for log.address === messenger.address
	for _, log := range receipt.Logs {
		if log.Address == i.lightlink.Opts.L2ToL1MessagePasserAddress {
			// get cross chain message for deposit so we can calculate message hash
			// we store the message hash in the db so we can check if it's been relayed on L2
			if log.Topics[0] == MessagePassedEventABIHash {
				err := contractAbi.UnpackIntoInterface(&message, "MessagePassed", log.Data)
				if err != nil {
					return nil, fmt.Errorf("failed to unpack MessagePassed: %w", err)
				}
			}
		}
	}

	hash := common.BytesToHash(message.WithdrawalHash[:])
	return &hash, nil
}
