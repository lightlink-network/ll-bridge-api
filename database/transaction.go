package database

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/lightlink-network/ll-bridge-api/database/models"
	"github.com/lightlink-network/ll-bridge-api/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *Database) GetTransactionByHash(ctx context.Context, hash string) (*models.Transaction, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions")

	var transaction models.Transaction
	err := collection.FindOne(ctx, bson.D{{Key: "tx_hash", Value: hash}}).Decode(&transaction)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("transaction not found")
		}
		return nil, fmt.Errorf("failed to get transaction by hash: %w", err)
	}

	return &transaction, nil
}

func (db *Database) GetTransactions(ctx context.Context, filter models.Filter, page int64, pageSize int64) (*models.PaginatedResult, error) {
	mongoFilter := buildFilter(filter)
	skip := (page - 1) * pageSize

	collection := db.client.Database(db.databaseName).Collection("transactions")
	totalCount, err := collection.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}

	// Create base filter without status but keeping all other filters
	baseFilter := bson.D{}
	for k, v := range mongoFilter {
		if k != "status" {
			baseFilter = append(baseFilter, bson.E{Key: k, Value: v})
		}
	}

	actionNeededFilter := bson.D{
		{Key: "$and", Value: bson.A{
			baseFilter, // Use baseFilter instead of mongoFilter
			bson.D{{Key: "status", Value: bson.M{
				"$in": []string{
					string(types.ReadyToProve),
					string(types.ReadyForRelay),
				},
			}}},
		}},
	}
	actionNeededCount, err := collection.CountDocuments(ctx, actionNeededFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get action needed count: %w", err)
	}

	pendingFilter := bson.D{
		{Key: "$and", Value: bson.A{
			baseFilter, // Use baseFilter instead of mongoFilter
			bson.D{{Key: "status", Value: bson.M{
				"$in": []string{
					string(types.UnconfirmedL1ToL2Message),
					string(types.StateRootNotPublished),
					string(types.InChallengePeriod),
				},
			}}},
		}},
	}
	pendingCount, err := collection.CountDocuments(ctx, pendingFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending count: %w", err)
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: mongoFilter}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "transactions_proven"},
			{Key: "localField", Value: "withdrawal_hash"},
			{Key: "foreignField", Value: "withdrawal_hash"},
			{Key: "as", Value: "prove_tx"},
		}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "transactions_finalized"},
			{Key: "localField", Value: "withdrawal_hash"},
			{Key: "foreignField", Value: "withdrawal_hash"},
			{Key: "as", Value: "finalize_tx"},
		}}},
		{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$prove_tx"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}},
		{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$finalize_tx"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "block_time", Value: -1}}}},
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: pageSize}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to execute aggregation: %w", err)
	}
	defer cursor.Close(ctx)

	var transactions []models.Transaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	return &models.PaginatedResult{
		Items:        transactions,
		TotalCount:   totalCount,
		Page:         page,
		PageSize:     pageSize,
		ActionNeeded: actionNeededCount,
		Pending:      pendingCount,
	}, nil
}

func (db *Database) BatchCreateTransactions(ctx context.Context, txs []models.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	operations := make([]mongo.WriteModel, len(txs))
	now := time.Now()

	for i, tx := range txs {
		filter := bson.D{
			{Key: "tx_hash", Value: tx.TxHash},
			{Key: "type", Value: tx.Type},
		}

		// Create a separate document for the main data
		mainDoc := bson.D{
			{Key: "type", Value: tx.Type},
			{Key: "erc20", Value: tx.ERC20},
			{Key: "status", Value: tx.Status},
			{Key: "tx_hash", Value: tx.TxHash},
			{Key: "block_number", Value: tx.BlockNumber},
			{Key: "block_hash", Value: tx.BlockHash},
			{Key: "block_time", Value: tx.BlockTime},
			{Key: "gas_used", Value: tx.GasUsed},
			{Key: "effective_gas_price", Value: tx.EffectiveGasPrice},
			{Key: "from", Value: tx.From},
			{Key: "to", Value: tx.To},
			{Key: "value", Value: tx.Value},
			{Key: "l1_token", Value: tx.L1Token},
			{Key: "l2_token", Value: tx.L2Token},
			{Key: "message", Value: tx.Message},
			{Key: "message_hash", Value: tx.MessageHash},
			{Key: "withdrawal_hash", Value: tx.WithdrawalHash},
		}

		update := bson.D{
			{Key: "$set", Value: mainDoc},
			{Key: "$setOnInsert", Value: bson.D{
				{Key: "created_at", Value: now},
			}},
			{Key: "$currentDate", Value: bson.D{
				{Key: "updated_at", Value: true},
			}},
		}

		operations[i] = mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)
	}

	collection := db.client.Database(db.databaseName).Collection("transactions")
	opts := options.BulkWrite().SetOrdered(false)

	_, err := collection.BulkWrite(ctx, operations, opts)
	if err != nil {
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			for _, writeErr := range bulkErr.WriteErrors {
				db.logger.Error("bulk write error",
					"index", writeErr.Index,
					"code", writeErr.Code,
					"message", writeErr.Message)
			}
		}
		return fmt.Errorf("failed to batch upsert transactions: %w", err)
	}

	return nil
}

func (db *Database) GetTransactionsByStatus(ctx context.Context, status string, txType string) ([]models.Transaction, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions")

	filter := bson.D{{Key: "status", Value: status}, {Key: "type", Value: txType}}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions by status: %w", err)
	}
	defer cursor.Close(ctx)

	var transactions []models.Transaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	return transactions, nil
}

func (db *Database) GetTransactionsProvenByStatus(ctx context.Context, status string) ([]models.TransactionProven, error) {
	transactions, err := db.GetTransactionsByStatus(ctx, status, "withdrawal")
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions by status: %w", err)
	}

	provenTransactions := make([]models.TransactionProven, len(transactions))
	for i, transaction := range transactions {
		provenTransactions[i] = models.TransactionProven{
			WithdrawalHash: transaction.WithdrawalHash,
		}
	}

	return provenTransactions, nil
}

// UpdateWithdrawalStatusByBlockNumber updates all withdrawals with status StateRootNotPublished to ReadyToProve
// if their block number is less than or equal to the given L2 block number
func (db *Database) UpdateWithdrawalsToReadyToProve(ctx context.Context, l2BlockNumber uint64) error {
	collection := db.client.Database(db.databaseName).Collection("transactions")

	filter := bson.D{
		{Key: "type", Value: "withdrawal"},
		{Key: "status", Value: types.StateRootNotPublished},
		{Key: "block_number", Value: bson.D{{Key: "$lte", Value: l2BlockNumber}}},
	}

	update := bson.D{{
		Key: "$set",
		Value: bson.D{{
			Key:   "status",
			Value: types.ReadyToProve,
		}},
	}}

	result, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update withdrawal statuses: %w", err)
	}

	if result.ModifiedCount > 0 {
		db.logger.Info("updated withdrawal statuses",
			"count", result.ModifiedCount,
			"l2BlockNumber", l2BlockNumber)
	}

	return nil
}

func (db *Database) UpdateTransactionStatusByWithdrawalHash(ctx context.Context, withdrawalHash string, status string) error {
	collection := db.client.Database(db.databaseName).Collection("transactions")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: status}}}}

	_, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	return nil
}

func (db *Database) UpdateTransactionByMessageHash(ctx context.Context, messageHash common.Hash, updates bson.D) error {
	update := bson.D{
		{Key: "$set", Value: append(updates,
			bson.E{Key: "updated_at", Value: time.Now()},
		)},
	}

	_, err := db.client.Database(db.databaseName).Collection("transactions").UpdateOne(
		ctx,
		bson.D{{Key: "message_hash", Value: messageHash.Hex()}},
		update,
	)

	return err
}
