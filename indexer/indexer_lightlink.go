package indexer

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lightlink-network/ll-bridge-api/database/models"
	"github.com/lightlink-network/ll-bridge-api/types"
)

func (i *Indexer) indexLightLink(ctx context.Context) error {
	minBatchSize := uint64(50)
	maxBatchSize := uint64(2000)
	start := i.lightlink.Opts.DefaultStartBlock

	// Get last indexed block
	lastIndexedBlock, err := i.database.GetLastIndexedBlock(context.Background(), "lightlink")
	if err == nil && lastIndexedBlock > 0 {
		start = lastIndexedBlock + 1
	}

	i.logger.Info("starting lightlink indexer", "startBlock", start)

	for {
		select {
		case <-ctx.Done():
			i.logger.Info("shutting down lightlink indexer")
			return nil
		default:
			// get current last block from chain rpc
			lastBlock, err := i.lightlink.BlockNumber()
			if err != nil {
				return fmt.Errorf("failed to get last block: %w", err)
			}

			// If we don't have enough blocks yet, wait and continue
			if lastBlock < start+minBatchSize {
				i.logger.Info("waiting for more lightlink blocks",
					"chainHead", lastBlock,
					"nextBatchStart", start)

				time.Sleep(time.Second * 10)
				continue
			}

			// Calculate batch size - use larger batches when catching up
			batchSize := uint64(min(maxBatchSize, lastBlock-start+1))
			end := start + batchSize - 1
			if end > lastBlock {
				end = lastBlock
			}

			i.logger.Info("processing lightlink blocks",
				"startBlock", start,
				"endBlock", end,
				"batchSize", end-start+1,
				"chainHead", lastBlock)

			// loop through blocks index withdrawals
			if err := i.indexL2Withdrawals(start, end); err != nil {
				return fmt.Errorf("failed to index withdrawals: %w", err)
			}

			if err := i.indexL2RelayedMessages(start, end); err != nil {
				return fmt.Errorf("failed to index relayed messages: %w", err)
			}

			i.logger.Info("lightlink batch complete",
				"blocksProcessed", end-start+1)

			// Update last indexed block
			if err := i.database.UpdateLastIndexedBlock(context.Background(), "lightlink", end); err != nil {
				return fmt.Errorf("failed to update last indexed block: %w", err)
			}

			// Move to next batch
			start = end + 1
		}
	}
}

func (i *Indexer) indexL2Withdrawals(startBlock uint64, endBlock uint64) error {
	withdrawals := make([]models.Withdrawal, 0)

	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	// Collect ETH withdrawals
	ethIter, err := i.lightlink.FilterETHBridgeInitiated(&opts, []common.Address{}, []common.Address{})
	if err != nil {
		return fmt.Errorf("failed to filter ETHBridgeInitiated: %w", err)
	}

	for ethIter.Next() {
		log := ethIter.Event

		message, err := i.txToCrossChainMessageV1(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to convert tx to cross chain message: %w", err)
		}
		// Now both event and value should be populated, can calculate message hash
		messageHash, err := HashCrossDomainMessageV1(message.MessageNonce, message.Sender, message.Target, message.Value, message.GasLimit, message.Message)
		if err != nil {
			return fmt.Errorf("failed to hash cross domain message: %w", err)
		}

		blockTime, err := i.lightlink.GetBlockTimestamp(log.Raw.BlockNumber)
		if err != nil {
			return fmt.Errorf("failed to get block: %w", err)
		}

		hash, err := i.getWithdrawalHash(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to hash withdrawal: %w", err)
		}

		withdrawals = append(withdrawals, models.Withdrawal{
			Type:           "withdrawal",
			ERC20:          false,
			From:           log.From.Hex(),
			To:             log.To.Hex(),
			Value:          log.Amount.String(),
			Message:        hex.EncodeToString(message.Message),
			MessageHash:    messageHash.Hex(),
			TxHash:         log.Raw.TxHash.Hex(),
			BlockNumber:    log.Raw.BlockNumber,
			BlockHash:      log.Raw.BlockHash.Hex(),
			BlockTime:      blockTime,
			WithdrawalHash: hash.Hex(),
			Status:         string(types.StateRootNotPublished),
		})

		i.logger.Info("withdrawal created", "tx_hash", log.Raw.TxHash.Hex())
	}

	// Collect ERC20 withdrawals
	erc20Iter, err := i.lightlink.FilterERC20BridgeInitiated(&opts, []common.Address{}, []common.Address{}, []common.Address{})
	if err != nil {
		return fmt.Errorf("failed to filter ERC20BridgeInitiated: %w", err)
	}

	for erc20Iter.Next() {
		log := erc20Iter.Event
		message, err := i.txToCrossChainMessageV1(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to convert tx to cross chain message: %w", err)
		}
		messageHash, err := HashCrossDomainMessageV1(message.MessageNonce, message.Sender, message.Target, message.Value, message.GasLimit, message.Message)
		if err != nil {
			return fmt.Errorf("failed to hash cross domain message: %w", err)
		}
		blockTime, err := i.lightlink.GetBlockTimestamp(log.Raw.BlockNumber)
		if err != nil {
			return fmt.Errorf("failed to get block: %w", err)
		}

		hash, err := i.getWithdrawalHash(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to hash withdrawal: %w", err)
		}

		withdrawals = append(withdrawals, models.Withdrawal{
			Type:           "withdrawal",
			ERC20:          true,
			From:           log.From.Hex(),
			To:             log.To.Hex(),
			Value:          log.Amount.String(),
			L1Token:        log.LocalToken.Hex(),
			L2Token:        log.RemoteToken.Hex(),
			Message:        hex.EncodeToString(message.Message),
			MessageHash:    messageHash.Hex(),
			TxHash:         log.Raw.TxHash.Hex(),
			BlockNumber:    log.Raw.BlockNumber,
			BlockHash:      log.Raw.BlockHash.Hex(),
			BlockTime:      blockTime,
			WithdrawalHash: hash.Hex(),
			Status:         string(types.StateRootNotPublished),
		})

		i.logger.Info("withdrawal created", "tx_hash", log.Raw.TxHash.Hex())
	}

	// Batch insert all withdrawals
	if err := i.database.BatchCreateWithdrawals(context.Background(), withdrawals); err != nil {
		return fmt.Errorf("failed to batch create withdrawals: %w", err)
	}

	return nil
}

func (i *Indexer) indexL2RelayedMessages(startBlock uint64, endBlock uint64) error {
	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	msgIter, err := i.lightlink.FilterRelayedMessage(&opts, [][32]byte{})
	if err != nil {
		return fmt.Errorf("failed to filter RelayedMessage: %w", err)
	}

	for msgIter.Next() {
		log := msgIter.Event
		i.logger.Info("L2RelayedMessage", "MsgHash", common.Hash(log.MsgHash).Hex())

		// Update deposit status to RELAYED using proper MongoDB update syntax
		if err := i.database.UpdateDepositStatus(context.Background(), common.Hash(log.MsgHash).Hex(), "RELAYED", log.Raw.TxHash.Hex()); err != nil {
			return fmt.Errorf("failed to update deposit status: %w", err)
		}
	}

	return nil
}
