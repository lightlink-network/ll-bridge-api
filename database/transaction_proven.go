package database

import (
	"context"
	"fmt"

	"github.com/lightlink-network/ll-bridge-api/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *Database) CreateTransactionProven(ctx context.Context, transaction models.TransactionProven) (string, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions_proven")

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
		return "", fmt.Errorf("failed to upsert transaction proven: %w", err)
	}

	if result.UpsertedID != nil {
		return result.UpsertedID.(primitive.ObjectID).Hex(), nil
	}
	return "", nil
}

// GetWithdrawalProvenByHash gets a withdrawal proven record by its withdrawal hash
func (db *Database) GetTransactionProvenByHash(ctx context.Context, withdrawalHash string) (models.TransactionProven, error) {
	collection := db.client.Database(db.databaseName).Collection("transactions_proven")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}

	var provenTransaction models.TransactionProven
	if err := collection.FindOne(ctx, filter).Decode(&provenTransaction); err != nil {
		return models.TransactionProven{}, fmt.Errorf("failed to get transaction proven by hash: %w", err)
	}

	return provenTransaction, nil
}
