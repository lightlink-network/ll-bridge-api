package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lightlink-network/ll-bridge-api/database/models"
	"github.com/lightlink-network/ll-bridge-api/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database struct {
	client       *mongo.Client
	databaseName string
	logger       *slog.Logger
}

type DatabaseOpts struct {
	URI          string
	DatabaseName string
	Logger       *slog.Logger
}

const (
	defaultBatchSize = 1000
	defaultTimeout   = 10 * time.Second
)

func NewDatabase(opts DatabaseOpts) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	clientOpts := options.Client().
		ApplyURI(opts.URI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetMaxConnecting(10).
		SetServerSelectionTimeout(5 * time.Second).
		SetRetryWrites(true)

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Database{
		client:       client,
		databaseName: opts.DatabaseName,
		logger:       opts.Logger,
	}, nil
}

func (db *Database) CreateIndexes(ctx context.Context) error {
	// Transactions collection indexes
	transactionsColl := db.client.Database(db.databaseName).Collection("transactions")
	_, err := transactionsColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tx_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "message_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "status", Value: 1},
				{Key: "block_time", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "withdrawal_hash", Value: 1},
				{Key: "status", Value: 1},
			},
		},
		{Keys: bson.D{{Key: "from", Value: 1}}},
		{Keys: bson.D{{Key: "to", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("failed to create transactions indexes: %w", err)
	}

	// Withdrawal proven collection indexes
	provenColl := db.client.Database(db.databaseName).Collection("transactions_proven")
	_, err = provenColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "withdrawal_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create transactions_proven index: %w", err)
	}

	// Withdrawal finalized collection indexes
	finalizedColl := db.client.Database(db.databaseName).Collection("transactions_finalized")
	_, err = finalizedColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "withdrawal_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create transactions_finalized index: %w", err)
	}

	return nil
}

func buildFilter(f models.Filter) bson.M {
	filter := bson.M{}

	// Handle special status filters
	switch f.Status {
	case "pending":
		filter["status"] = bson.M{
			"$in": []string{
				string(types.UnconfirmedL1ToL2Message),
				string(types.StateRootNotPublished),
				string(types.InChallengePeriod),
			},
		}
	case "action-needed":
		filter["status"] = bson.M{
			"$in": []string{
				string(types.ReadyToProve),
				string(types.ReadyForRelay),
			},
		}
	case "":
		// No status filter
	default:
		// Regular status filter
		filter["status"] = f.Status
	}

	// Add other filters
	if f.From != "" {
		filter["from"] = f.From
	}
	if f.To != "" {
		filter["to"] = f.To
	}
	if f.TxHash != "" {
		filter["tx_hash"] = f.TxHash
	}
	if f.Type != "" {
		filter["type"] = f.Type
	}
	return filter
}
