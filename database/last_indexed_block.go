package database

import (
	"context"
	"fmt"
	"log"

	"github.com/lightlink-network/ll-bridge-api/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Update or create store the last indexed block for ethereum or/and lightlink
func (db *Database) UpdateLastIndexedBlock(ctx context.Context, chain string, blockNumber uint64) error {
	collection := db.client.Database(db.databaseName).Collection("last_indexed_block")

	filter := bson.D{{Key: "chain", Value: chain}}
	update := bson.D{{
		Key: "$set",
		Value: bson.D{{
			Key: "block_number", Value: blockNumber,
		}},
	}}

	_, err := collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("failed to update last indexed block: %w", err)
	}

	return nil
}

func (db *Database) GetLastIndexedBlock(ctx context.Context, chain string) (uint64, error) {
	collection := db.client.Database(db.databaseName).Collection("last_indexed_block")

	var result models.LastIndexedBlock
	err := collection.FindOne(ctx, bson.D{{Key: "chain", Value: chain}}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get last indexed block: %w", err)
	}

	log.Printf("last indexed block for chain %s: %+v", chain, result)

	return result.BlockNumber, nil
}
