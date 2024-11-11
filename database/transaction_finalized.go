package database

import (
	"context"
	"fmt"

	"github.com/lightlink-network/ll-bridge-api/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *Database) CreateTransactionFinalized(ctx context.Context, transaction models.TransactionFinalized) (string, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions_finalized")

	filter := bson.D{{Key: "withdrawal_hash", Value: transaction.WithdrawalHash}}
	update := bson.D{{
		Key:   "$set",
		Value: transaction,
	}}

	result, err := collection.UpdateOne(
		ctx,
		filter,
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to upsert transaction finalized: %w", err)
	}

	if result.UpsertedID != nil {
		return result.UpsertedID.(primitive.ObjectID).Hex(), nil
	}
	return "", nil
}

// GetTransactionFinalizedByHash gets a transaction finalized record by its withdrawal hash
func (db *Database) GetTransactionFinalizedByHash(ctx context.Context, withdrawalHash string) (models.TransactionFinalized, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions_finalized")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}

	var finalizedTransaction models.TransactionFinalized
	if err := collection.FindOne(ctx, filter).Decode(&finalizedTransaction); err != nil {
		return models.TransactionFinalized{}, fmt.Errorf("failed to get transaction finalized by hash: %w", err)
	}

	return finalizedTransaction, nil
}
