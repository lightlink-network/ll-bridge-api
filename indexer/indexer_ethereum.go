package indexer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lightlink-network/ll-bridge-api/database/models"
	"github.com/lightlink-network/ll-bridge-api/types"
	"go.mongodb.org/mongo-driver/mongo"
)

func (i *Indexer) indexEthereum(ctx context.Context) error {
	minBatchSize := i.ethereum.Opts.MinBatchSize
	maxBatchSize := i.ethereum.Opts.MaxBatchSize
	start := i.ethereum.Opts.DefaultStartBlock

	// Get last indexed block
	lastIndexedBlock, err := i.database.GetLastIndexedBlock(context.Background(), "ethereum")
	if err == nil && lastIndexedBlock > 0 {
		start = lastIndexedBlock + 1
	}

	go i.CheckWithdrawalFinalizedStatus(ctx)
	go i.CheckWithdrawalProvenStatus(ctx)

	i.logger.Info("starting ethereum indexer", "startBlock", start)

	for {
		select {
		case <-ctx.Done():
			i.logger.Info("shutting down ethereum indexer")
			return nil
		default:
			// Get current block
			lastBlock, err := i.ethereum.BlockNumber()
			if err != nil {
				return fmt.Errorf("failed to get current block: %w", err)
			}

			// If we don't have enough blocks yet, wait and continue
			if lastBlock < start+minBatchSize {
				i.logger.Info("waiting for more ethereum blocks",
					"chainHead", lastBlock,
					"nextBatchStart", start,
					"minBatchSize", minBatchSize)
				time.Sleep(time.Duration(i.ethereum.Opts.FetchInterval) * time.Second)
				continue
			}

			// Calculate batch size - use larger batches when catching up
			batchSize := uint64(min(maxBatchSize, lastBlock-start+1))
			end := start + batchSize - 1
			if end > lastBlock {
				end = lastBlock
			}

			i.logger.Info("processing ethereum blocks",
				"startBlock", start,
				"endBlock", end,
				"batchSize", end-start+1,
				"chainHead", lastBlock)

			// Process batch
			if err := i.indexL1Deposits(start, end); err != nil {
				return fmt.Errorf("failed to index L1 deposits: %w", err)
			}

			if err := i.indexL1FilterWithdrawalProven(start, end); err != nil {
				i.logger.Error("failed to index L1 withdrawal proven", "error", err)
				return fmt.Errorf("failed to index L1 withdrawal proven: %w", err)
			}

			if err := i.indexL1FilterWithdrawalFinalized(start, end); err != nil {
				i.logger.Error("failed to index L1 withdrawal finalized", "error", err)
				return fmt.Errorf("failed to index L1 withdrawal finalized: %w", err)
			}

			if err := i.indexL2OutputProposed(start, end); err != nil {
				i.logger.Error("failed to index L2 output proposed", "error", err)
				return fmt.Errorf("failed to index L2 output proposed: %w", err)
			}

			// Update last indexed block
			if err := i.database.UpdateLastIndexedBlock(context.Background(), "ethereum", end); err != nil {
				return fmt.Errorf("failed to update last indexed block: %w", err)
			}

			i.logger.Info("ethereum batch complete", "blocksProcessed", end-start+1)

			start = end + 1
		}
	}
}

func (i *Indexer) indexL1Deposits(startBlock uint64, endBlock uint64) error {
	deposits := make([]models.Transaction, 0)

	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	// Index ETH deposits
	ethIter, err := i.ethereum.FilterETHBridgeInitiated(&opts, []common.Address{}, []common.Address{})
	if err != nil {
		return fmt.Errorf("failed to filter ETHBridgeInitiated: %w", err)
	}

	for ethIter.Next() {
		log := ethIter.Event

		message, gasUsed, err := i.txToCrossChainMessageV1(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to convert tx to cross chain message: %w", err)
		}
		messageHash, err := HashCrossDomainMessageV1(message.MessageNonce, message.Sender, message.Target, message.Value, message.GasLimit, message.Message)
		if err != nil {
			return fmt.Errorf("failed to hash cross domain message: %w", err)
		}
		block, err := i.ethereum.BlockByNumber(context.Background(), big.NewInt(int64(log.Raw.BlockNumber)))
		if err != nil {
			return fmt.Errorf("failed to get block: %w", err)
		}

		deposits = append(deposits, models.Transaction{
			Type:        "deposit",
			ERC20:       false,
			From:        log.From.Hex(),
			To:          log.To.Hex(),
			Value:       log.Amount.String(),
			Message:     hex.EncodeToString(message.Message),
			MessageHash: messageHash.Hex(),
			TxHash:      log.Raw.TxHash.Hex(),
			BlockNumber: log.Raw.BlockNumber,
			BlockHash:   log.Raw.BlockHash.Hex(),
			BlockTime:   block.Time(),
			GasUsed:     *gasUsed,
			Status:      string(types.UnconfirmedL1ToL2Message),
		})
	}

	// Index ERC20 deposits
	erc20Iter, err := i.ethereum.FilterERC20BridgeInitiated(&opts, []common.Address{}, []common.Address{}, []common.Address{})
	if err != nil {
		return fmt.Errorf("failed to filter ERC20BridgeInitiated: %w", err)
	}

	for erc20Iter.Next() {
		log := erc20Iter.Event

		message, gasUsed, err := i.txToCrossChainMessageV1(log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to convert tx to cross chain message: %w", err)
		}
		messageHash, err := HashCrossDomainMessageV1(message.MessageNonce, message.Sender, message.Target, message.Value, message.GasLimit, message.Message)
		if err != nil {
			return fmt.Errorf("failed to hash cross domain message: %w", err)
		}
		block, err := i.ethereum.BlockByNumber(context.Background(), big.NewInt(int64(log.Raw.BlockNumber)))
		if err != nil {
			return fmt.Errorf("failed to get block: %w", err)
		}

		deposits = append(deposits, models.Transaction{
			Type:        "deposit",
			ERC20:       true,
			From:        log.From.Hex(),
			To:          log.To.Hex(),
			Value:       log.Amount.String(),
			L1Token:     log.LocalToken.Hex(),
			L2Token:     log.RemoteToken.Hex(),
			Message:     hex.EncodeToString(message.Message),
			MessageHash: messageHash.Hex(),
			TxHash:      log.Raw.TxHash.Hex(),
			BlockNumber: log.Raw.BlockNumber,
			BlockHash:   log.Raw.BlockHash.Hex(),
			BlockTime:   block.Time(),
			GasUsed:     *gasUsed,
			Status:      string(types.UnconfirmedL1ToL2Message),
		})
	}

	// Batch insert all deposits
	if err := i.database.BatchCreateTransactions(context.Background(), deposits); err != nil {
		return fmt.Errorf("failed to batch create deposits: %w", err)
	}

	return nil
}

func (i *Indexer) indexL1FilterWithdrawalProven(startBlock uint64, endBlock uint64) error {
	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	iter, err := i.ethereum.FilterWithdrawalProven(&opts, [][32]byte{}, []common.Address{}, []common.Address{})
	if err != nil {
		return fmt.Errorf("failed to filter withdrawal proven: %w", err)
	}

	for iter.Next() {
		log := iter.Event
		i.logger.Info("withdrawal proven", "withdrawalHash", common.Hash(log.WithdrawalHash).Hex())

		// if a withdrawal exists with a status of ReadyToProve, update its status to InChallengePeriod, if it doesnt exit dont error
		if err := i.database.UpdateTransactionStatusByWithdrawalHash(context.Background(), common.Hash(log.WithdrawalHash).Hex(), string(types.InChallengePeriod)); err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return fmt.Errorf("failed to update withdrawal status: %w", err)
			}
		}

		// get proven withdrawal timestamp and l2OutputIndex from csc
		outputRoot, err := i.ethereum.ProvenWithdrawals(nil, log.WithdrawalHash)
		if err != nil {
			return fmt.Errorf("failed to get proven withdrawals: %w", err)
		}

		// get gas used from tx receipt
		receipt, err := i.ethereum.TransactionReceipt(context.Background(), log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to get transaction receipt: %w", err)
		}

		// Create withdrawal proven record
		if _, err := i.database.CreateTransactionProven(context.Background(), models.TransactionProven{
			WithdrawalHash: common.Hash(log.WithdrawalHash).Hex(),
			TxHash:         log.Raw.TxHash.Hex(),
			BlockNumber:    log.Raw.BlockNumber,
			Timestamp:      outputRoot.Timestamp.Uint64(),
			L2OutputIndex:  outputRoot.L2OutputIndex.Uint64(),
			GasUsed:        receipt.GasUsed,
		}); err != nil {
			return fmt.Errorf("failed to create withdrawal proven: %w", err)
		}
	}

	return nil
}

func (i *Indexer) indexL1FilterWithdrawalFinalized(startBlock uint64, endBlock uint64) error {
	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	iter, err := i.ethereum.FilterWithdrawalFinalized(&opts, [][32]byte{})
	if err != nil {
		return fmt.Errorf("failed to filter withdrawal finalized: %w", err)
	}

	for iter.Next() {
		log := iter.Event
		i.logger.Info("withdrawal finalized", "withdrawalHash", common.Hash(log.WithdrawalHash).Hex())

		// if a withdrawal exists with a status of ReadyForRelay, update its status to Relayed, if it doesnt exit dont error
		if err := i.database.UpdateTransactionStatusByWithdrawalHash(context.Background(), common.Hash(log.WithdrawalHash).Hex(), string(types.Relayed)); err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return fmt.Errorf("failed to update withdrawal status: %w", err)
			}
		}

		// Get tx block time
		block, err := i.ethereum.BlockByNumber(context.Background(), big.NewInt(int64(log.Raw.BlockNumber)))
		if err != nil {
			return fmt.Errorf("failed to get block: %w", err)
		}

		// get gas used from tx receipt
		receipt, err := i.ethereum.TransactionReceipt(context.Background(), log.Raw.TxHash)
		if err != nil {
			return fmt.Errorf("failed to get transaction receipt: %w", err)
		}

		// Create withdrawal finalized
		if _, err := i.database.CreateTransactionFinalized(context.Background(), models.TransactionFinalized{
			WithdrawalHash: common.Hash(log.WithdrawalHash).Hex(),
			TxHash:         log.Raw.TxHash.Hex(),
			BlockNumber:    log.Raw.BlockNumber,
			Timestamp:      block.Time(),
			GasUsed:        receipt.GasUsed,
		}); err != nil {
			return fmt.Errorf("failed to create withdrawal finalized: %w", err)
		}
	}

	return nil
}

// Every time an output is proposed, we check if any withdrawals awaiting an output root are ready to be proven
// We also need to check if any of the withdrawals that are in the challenge period can now be relayed/finalized
func (i *Indexer) indexL2OutputProposed(startBlock uint64, endBlock uint64) error {
	opts := bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}

	iter, err := i.ethereum.FilterOutputProposed(&opts, [][32]byte{}, []*big.Int{}, []*big.Int{})
	if err != nil {
		return fmt.Errorf("failed to filter output proposed: %w", err)
	}

	for iter.Next() {
		log := iter.Event
		i.logger.Info("output proposed", "outputRoot", common.Hash(log.OutputRoot).Hex(), "l2BlockNumber", log.L2BlockNumber.Uint64())

		// Update all withdrawals with a status of StateRootNotPublished to ReadyToProve IF the log.L2BlockNumber >= the withdrawal's L2BlockNumber
		// This is safe because the L2BlockNumber can never go down, only up as more outputs are proposed
		if err := i.database.UpdateWithdrawalsToReadyToProve(context.Background(), log.L2BlockNumber.Uint64()); err != nil {
			return fmt.Errorf("failed to update withdrawal statuses to ready to prove: %w", err)
		}

		// get all proven withdrawals that correspond to the withdrawals with a status of InChallengePeriod
		provenWithdrawals, err := i.database.GetTransactionsProvenByStatus(context.Background(), string(types.InChallengePeriod))
		if err != nil {
			return fmt.Errorf("failed to get proven withdrawals: %w", err)
		}

		// calls LLPortal isOutputFinalized function for each withdrawal that has a status of InChallengePeriod and if true sets status to ReadyToRelay
		for _, withdrawal := range provenWithdrawals {
			isFinalized, err := i.ethereum.IsOutputFinalized(nil, big.NewInt(int64(withdrawal.L2OutputIndex)))
			if err != nil {
				return fmt.Errorf("failed to check if output is finalized: %w", err)
			}

			if isFinalized {
				if err := i.database.UpdateTransactionStatusByWithdrawalHash(context.Background(), withdrawal.WithdrawalHash, string(types.ReadyForRelay)); err != nil {
					return fmt.Errorf("failed to update withdrawal status: %w", err)
				}
			}
		}
	}

	return nil
}

// Run every 5 minutes and get all withdrawals with a status of ReadyToProve, checks if a matching withdrawals_proven record exists for the withdrawal
// If it does, it sets the withdrawal status to InChallengePeriod
func (i *Indexer) CheckWithdrawalProvenStatus(ctx context.Context) error {
	// Run in a loop with a 5-minute sleep interval
	for {
		select {
		case <-ctx.Done():
			i.logger.Info("shutting down withdrawal proven status checker")
			return nil
		default:
			// Get all withdrawals with ReadyToProve status
			withdrawals, err := i.database.GetTransactionsByStatus(ctx, string(types.ReadyToProve), "withdrawal")
			if err != nil {
				return fmt.Errorf("failed to get withdrawals with ReadyToProve status: %w", err)
			}

			for _, withdrawal := range withdrawals {
				// Get withdrawal proven record for this withdrawal
				proven, err := i.database.GetTransactionProvenByHash(ctx, withdrawal.WithdrawalHash)
				if err != nil {
					// If error is not "not found", return the error
					if !errors.Is(err, mongo.ErrNoDocuments) {
						return fmt.Errorf("failed to get withdrawal proven record: %w", err)
					}
					// If withdrawal proven not found, continue to next withdrawal
					continue
				}

				// If we found a matching withdrawals_proven record, update the withdrawal status
				if proven.WithdrawalHash != "" {
					if err := i.database.UpdateTransactionStatusByWithdrawalHash(ctx, withdrawal.WithdrawalHash, string(types.InChallengePeriod)); err != nil {
						return fmt.Errorf("failed to update withdrawal status: %w", err)
					}
					i.logger.Info("updated withdrawal status to InChallengePeriod",
						"withdrawalHash", withdrawal.WithdrawalHash)
				}
			}

			// Sleep for StatusCheckInterval seconds before next check
			time.Sleep(time.Duration(i.ethereum.Opts.StatusCheckInterval) * time.Second)
		}
	}
}

// Run every 5 minutes and get all withdrawals with a status of ReadyForRelay, checks if a matching withdrawals_finalized record exists for the withdrawal
// If it does, it sets the withdrawal status to Relayed
// This is to prevent a situation where the L2 output is proposed and the withdrawal is proven, but the withdrawals_finalized record is not submitted in time and the withdrawal is not relayed/finalized
func (i *Indexer) CheckWithdrawalFinalizedStatus(ctx context.Context) error {
	// Run in a loop with a sleep interval
	for {
		select {
		case <-ctx.Done():
			i.logger.Info("shutting down withdrawal finalized status checker")
			return nil
		default:
			// Get all withdrawals with ReadyForRelay status
			withdrawals, err := i.database.GetTransactionsByStatus(ctx, string(types.ReadyForRelay), "withdrawal")
			if err != nil {
				return fmt.Errorf("failed to get withdrawals with ReadyForRelay status: %w", err)
			}

			for _, withdrawal := range withdrawals {
				// Get withdrawal finalized record for this withdrawal
				finalized, err := i.database.GetTransactionFinalizedByHash(ctx, withdrawal.WithdrawalHash)
				if err != nil {
					// If error is not "not found", return the error
					if !errors.Is(err, mongo.ErrNoDocuments) {
						return fmt.Errorf("failed to get withdrawal finalized record: %w", err)
					}
					// If withdrawal finalized not found, continue to next withdrawal
					continue
				}

				// If we found a matching withdrawals_finalized record, update the withdrawal status
				if finalized.WithdrawalHash != "" {
					if err := i.database.UpdateTransactionStatusByWithdrawalHash(ctx, withdrawal.WithdrawalHash, string(types.Relayed)); err != nil {
						return fmt.Errorf("failed to update withdrawal status: %w", err)
					}
					i.logger.Info("updated withdrawal status to Relayed",
						"withdrawalHash", withdrawal.WithdrawalHash)
				}
			}

			// Sleep for StatusCheckInterval seconds before next check
			time.Sleep(time.Duration(i.ethereum.Opts.StatusCheckInterval) * time.Second)
		}
	}
}
