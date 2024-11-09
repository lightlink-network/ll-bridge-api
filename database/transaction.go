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

	opts := options.Find().
		SetHint(bson.D{{Key: "tx_hash", Value: 1}}).
		SetBatchSize(1)

	cursor, err := collection.Find(ctx, bson.D{{Key: "tx_hash", Value: hash}}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction by hash: %w", err)
	}
	defer cursor.Close(ctx)

	var transaction models.Transaction
	if err := cursor.Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %w", err)
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
		Items:      transactions,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func (db *Database) BatchCreateTransactions(ctx context.Context, txs []models.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	now := time.Now()
	documents := make([]interface{}, len(txs))
	for i, tx := range txs {
		tx.CreatedAt = now
		tx.UpdatedAt = now
		documents[i] = tx
	}

	_, err := db.client.Database(db.databaseName).Collection("transactions").InsertMany(
		ctx,
		documents,
		options.InsertMany().SetOrdered(false),
	)

	if err != nil {
		if writeErr, ok := err.(mongo.BulkWriteException); ok {
			// Handle duplicate key errors
			for _, writeError := range writeErr.WriteErrors {
				if writeError.Code != 11000 { // Duplicate key error code
					return err
				}
			}
			return nil
		}
		return err
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
