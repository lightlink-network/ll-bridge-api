package database

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/lightlink-network/ll-bridge-api/database/models"
	"github.com/lightlink-network/ll-bridge-api/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		SetMaxPoolSize(100).  // Adjust based on your needs
		SetMinPoolSize(10).   // Maintain minimum connections
		SetMaxConnecting(10). // Limit concurrent connection attempts
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
	// Deposits collection indexes
	depositsColl := db.client.Database(db.databaseName).Collection("deposits")
	_, err := depositsColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tx_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "message_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "block_time", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "from", Value: 1}}},
		{Keys: bson.D{{Key: "to", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("failed to create deposits indexes: %w", err)
	}

	// Withdrawals collection indexes
	withdrawalsColl := db.client.Database(db.databaseName).Collection("withdrawals")
	_, err = withdrawalsColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tx_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "withdrawal_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "block_time", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "from", Value: 1}}},
		{Keys: bson.D{{Key: "to", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("failed to create withdrawals indexes: %w", err)
	}

	// Withdrawal proven/finalized indexes
	provenColl := db.client.Database(db.databaseName).Collection("withdrawals_proven")
	_, err = provenColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "withdrawal_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create withdrawals_proven index: %w", err)
	}

	finalizedColl := db.client.Database(db.databaseName).Collection("withdrawals_finalized")
	_, err = finalizedColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "withdrawal_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create withdrawals_finalized index: %w", err)
	}

	return nil
}

func (db *Database) UpdateDepositStatus(ctx context.Context, messageHash string, status string, txHash string) error {
	filter := bson.D{{Key: "message_hash", Value: messageHash}}
	update := bson.D{{
		Key: "$set",
		Value: bson.D{{
			Key:   "status",
			Value: status,
		}, {
			Key:   "l1_tx_hash",
			Value: txHash,
		}},
	}}

	result, err := db.client.Database(db.databaseName).Collection("deposits").UpdateOne(
		ctx,
		filter,
		update,
	)
	if err != nil {
		return fmt.Errorf("failed to update deposit status: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("no deposit found with messageHash: %s", messageHash)
	}

	return nil
}

// update or create store the last indexed block for ethereum or/and lightlink
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
			// No document found - this chain hasn't been indexed yet
			return 0, nil // or return 0, fmt.Errorf("no indexed blocks found for chain %s", chain)
		}
		return 0, fmt.Errorf("failed to get last indexed block: %w", err)
	}

	log.Printf("last indexed block for chain %s: %+v", chain, result)

	return result.BlockNumber, nil
}

// BatchCreateDeposits creates multiple deposits in a single transaction
func (db *Database) BatchCreateDeposits(ctx context.Context, deposits []models.Deposit) error {
	if len(deposits) == 0 {
		return nil
	}

	collection := db.client.Database(db.databaseName).Collection("deposits")
	documents := make([]interface{}, len(deposits))
	for i, deposit := range deposits {
		documents[i] = deposit
	}

	_, err := collection.InsertMany(ctx, documents, options.InsertMany().SetOrdered(false))
	if err != nil {
		// Check if the error is due to duplicate keys
		if writeErr, ok := err.(mongo.BulkWriteException); ok {
			// Count successful inserts
			successfulInserts := len(deposits) - len(writeErr.WriteErrors)
			if successfulInserts > 0 {
				db.logger.Info("partially inserted deposits",
					"successful", successfulInserts,
					"failed", len(writeErr.WriteErrors))
			}
			// If all errors are duplicate key errors, don't return an error
			allDuplicates := true
			for _, writeErr := range writeErr.WriteErrors {
				if writeErr.Code != 11000 { // 11000 is MongoDB's duplicate key error code
					allDuplicates = false
					break
				}
			}
			if allDuplicates {
				return nil
			}
		}
		return fmt.Errorf("failed to insert deposits: %w", err)
	}

	return nil
}

// BatchCreateWithdrawals creates multiple withdrawals in a single transaction
func (db *Database) BatchCreateWithdrawals(ctx context.Context, withdrawals []models.Withdrawal) error {
	if len(withdrawals) == 0 {
		return nil
	}

	collection := db.client.Database(db.databaseName).Collection("withdrawals")
	documents := make([]interface{}, len(withdrawals))
	for i, withdrawal := range withdrawals {
		documents[i] = withdrawal
	}

	_, err := collection.InsertMany(ctx, documents, options.InsertMany().SetOrdered(false))
	if err != nil {
		// Check if the error is due to duplicate keys
		if writeErr, ok := err.(mongo.BulkWriteException); ok {
			// Count successful inserts
			successfulInserts := len(withdrawals) - len(writeErr.WriteErrors)
			if successfulInserts > 0 {
				db.logger.Info("partially inserted withdrawals",
					"successful", successfulInserts,
					"failed", len(writeErr.WriteErrors))
			}
			// If all errors are duplicate key errors, don't return an error
			allDuplicates := true
			for _, writeErr := range writeErr.WriteErrors {
				if writeErr.Code != 11000 { // 11000 is MongoDB's duplicate key error code
					allDuplicates = false
					break
				}
			}
			if allDuplicates {
				return nil
			}
		}
		return fmt.Errorf("failed to insert withdrawals: %w", err)
	}

	return nil
}

func buildFilter(f models.Filter) bson.M {
	filter := bson.M{}
	if f.Status != "" {
		filter["status"] = f.Status
	}
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

func (db *Database) CreateWithdrawalProven(ctx context.Context, withdrawal models.WithdrawalProven) (string, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals_proven")

	result, err := collection.InsertOne(ctx, withdrawal)
	if err != nil {
		// Check if error is due to duplicate key
		if mongo.IsDuplicateKeyError(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to create withdrawal proven: %w", err)
	}

	return result.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (db *Database) CreateWithdrawalFinalized(ctx context.Context, withdrawal models.WithdrawalFinalized) (string, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals_finalized")

	result, err := collection.InsertOne(ctx, withdrawal)
	if err != nil {
		// Check if error is due to duplicate key
		if mongo.IsDuplicateKeyError(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to create withdrawal finalized: %w", err)
	}

	return result.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (db *Database) GetTransactionByHash(ctx context.Context, hash string) (models.Transaction, error) {
	collection := db.client.Database(db.databaseName).Collection("deposits")

	// Pipeline to transform and combine deposits and withdrawals
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "tx_hash", Value: hash}}}},
		{{Key: "$addFields", Value: bson.D{
			{Key: "type", Value: "deposit"},
			{Key: "withdrawal_hash", Value: nil},
			{Key: "prove_tx", Value: nil},
			{Key: "finalize_tx", Value: nil},
		}}},
		{{Key: "$unionWith", Value: bson.D{
			{Key: "coll", Value: "withdrawals"},
			{Key: "pipeline", Value: mongo.Pipeline{
				{{Key: "$match", Value: bson.D{{Key: "tx_hash", Value: hash}}}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "withdrawals_proven"},
					{Key: "localField", Value: "withdrawal_hash"},
					{Key: "foreignField", Value: "withdrawal_hash"},
					{Key: "as", Value: "prove_tx"},
				}}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "withdrawals_finalized"},
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
				{{Key: "$addFields", Value: bson.D{{Key: "type", Value: "withdrawal"}}}},
			}},
		}}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return models.Transaction{}, fmt.Errorf("failed to execute transaction aggregation: %w", err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return models.Transaction{}, fmt.Errorf("failed to decode transaction: %w", err)
	}

	if len(results) == 0 {
		return models.Transaction{}, mongo.ErrNoDocuments
	}

	txMap := results[0]
	transaction := models.Transaction{
		Type:        txMap["type"].(string),
		ERC20:       txMap["erc20"].(bool),
		From:        txMap["from"].(string),
		To:          txMap["to"].(string),
		Value:       txMap["value"].(string),
		L1Token:     txMap["l1_token"].(string),
		L2Token:     txMap["l2_token"].(string),
		Message:     txMap["message"].(string),
		MessageHash: txMap["message_hash"].(string),
		TxHash:      txMap["tx_hash"].(string),
		BlockNumber: uint64(txMap["block_number"].(int64)),
		BlockHash:   txMap["block_hash"].(string),
		BlockTime:   uint64(txMap["block_time"].(int64)),
		Status:      txMap["status"].(string),
	}

	if txMap["type"].(string) == "withdrawal" {
		transaction.WithdrawalHash = txMap["withdrawal_hash"].(string)
		if txMap["prove_tx"] != nil {
			transaction.ProvenTx = &models.WithdrawalProven{}
			raw, _ := bson.Marshal(txMap["prove_tx"])
			bson.Unmarshal(raw, transaction.ProvenTx)
		}
		if txMap["finalize_tx"] != nil {
			transaction.FinalizeTx = &models.WithdrawalFinalized{}
			raw, _ := bson.Marshal(txMap["finalize_tx"])
			bson.Unmarshal(raw, transaction.FinalizeTx)
		}
	} else {
		if l1TxHash, ok := txMap["l1_tx_hash"]; ok {
			transaction.L1TxHash = l1TxHash.(string)
		}
	}

	return transaction, nil
}

func (db *Database) GetTransactions(ctx context.Context, filter models.Filter, page, pageSize int64) (*models.PaginatedResult, error) {
	collection := db.client.Database(db.databaseName).Collection("deposits")

	mongoFilter := buildFilter(filter)
	skip := (page - 1) * pageSize

	// Pipeline to transform and combine deposits and withdrawals
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: mongoFilter}},
		{{Key: "$sort", Value: bson.D{{Key: "block_time", Value: -1}}}}, // Move sort before union
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: pageSize}},
		{{Key: "$addFields", Value: bson.D{
			{Key: "type", Value: "deposit"},
			{Key: "withdrawal_hash", Value: nil},
			{Key: "prove_tx", Value: nil},
			{Key: "finalize_tx", Value: nil},
		}}},
		{{Key: "$unionWith", Value: bson.D{
			{Key: "coll", Value: "withdrawals"},
			{Key: "pipeline", Value: mongo.Pipeline{
				{{Key: "$match", Value: mongoFilter}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "withdrawals_proven"},
					{Key: "localField", Value: "withdrawal_hash"},
					{Key: "foreignField", Value: "withdrawal_hash"},
					{Key: "as", Value: "prove_tx"},
				}}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "withdrawals_finalized"},
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
				{{Key: "$addFields", Value: bson.D{{Key: "type", Value: "withdrawal"}}}},
			}},
		}}},
		{{Key: "$facet", Value: bson.D{
			{Key: "metadata", Value: bson.A{
				bson.D{{Key: "$count", Value: "total"}},
			}},
			{Key: "transactions", Value: bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "block_time", Value: -1}}}},
				bson.D{{Key: "$skip", Value: skip}},
				bson.D{{Key: "$limit", Value: pageSize}},
			}},
		}}},
	}

	// Add read preference for better performance
	opts := options.Aggregate().
		SetMaxTime(30 * time.Second).
		SetBatchSize(1000).
		SetAllowDiskUse(true)

	var result []bson.M
	cursor, err := collection.Aggregate(ctx, pipeline, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to execute transaction aggregation: %w", err)
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &result); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	if len(result) == 0 {
		return &models.PaginatedResult{
			Items:      []interface{}{},
			TotalCount: 0,
			Page:       page,
			PageSize:   pageSize,
		}, nil
	}

	facetResult := result[0]
	metadata := facetResult["metadata"].(primitive.A)
	transactions := facetResult["transactions"].(primitive.A)

	totalCount := int32(0)
	if len(metadata) > 0 {
		totalCount = metadata[0].(bson.M)["total"].(int32)
	}

	// Convert transactions to the common Transaction model
	transactionResults := make([]interface{}, len(transactions))
	for i, t := range transactions {
		txMap := t.(bson.M)
		transaction := models.Transaction{
			Type:        txMap["type"].(string),
			ERC20:       txMap["erc20"].(bool),
			From:        txMap["from"].(string),
			To:          txMap["to"].(string),
			Value:       txMap["value"].(string),
			L1Token:     txMap["l1_token"].(string),
			L2Token:     txMap["l2_token"].(string),
			Message:     txMap["message"].(string),
			MessageHash: txMap["message_hash"].(string),
			TxHash:      txMap["tx_hash"].(string),
			BlockNumber: uint64(txMap["block_number"].(int64)),
			BlockHash:   txMap["block_hash"].(string),
			BlockTime:   uint64(txMap["block_time"].(int64)),
			Status:      txMap["status"].(string),
		}

		if txMap["type"].(string) == "withdrawal" {
			transaction.WithdrawalHash = txMap["withdrawal_hash"].(string)
			if txMap["prove_tx"] != nil {
				transaction.ProvenTx = &models.WithdrawalProven{}
				raw, _ := bson.Marshal(txMap["prove_tx"])
				bson.Unmarshal(raw, transaction.ProvenTx)
			}
			if txMap["finalize_tx"] != nil {
				transaction.FinalizeTx = &models.WithdrawalFinalized{}
				raw, _ := bson.Marshal(txMap["finalize_tx"])
				bson.Unmarshal(raw, transaction.FinalizeTx)
			}
		} else {
			if l1TxHash, ok := txMap["l1_tx_hash"]; ok {
				transaction.L1TxHash = l1TxHash.(string)
			}
		}

		transactionResults[i] = transaction
	}

	return &models.PaginatedResult{
		Items:      transactionResults,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// UpdateWithdrawalStatusByBlockNumber updates all withdrawals with status StateRootNotPublished to ReadyToProve
// if their block number is less than or equal to the given L2 block number
func (db *Database) UpdateWithdrawalsToReadyToProve(ctx context.Context, l2BlockNumber uint64) error {
	collection := db.client.Database(db.databaseName).Collection("withdrawals")

	filter := bson.D{
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

func (db *Database) GetWithdrawalsByStatus(ctx context.Context, status string) ([]models.Withdrawal, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals")

	opts := options.Find().
		SetHint(bson.D{{Key: "status", Value: 1}}).
		SetBatchSize(1000)

	cursor, err := collection.Find(ctx, bson.D{{Key: "status", Value: status}}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawals by status: %w", err)
	}
	defer cursor.Close(ctx)

	var withdrawals []models.Withdrawal
	if err := cursor.All(ctx, &withdrawals); err != nil {
		return nil, fmt.Errorf("failed to decode withdrawals: %w", err)
	}

	return withdrawals, nil
}

// GetWithdrawalsProvenByStatus gets all proven withdrawals that correspond to the 'withdrawals' collection Swith a status of InChallengePeriod
func (db *Database) GetWithdrawalsProvenByStatus(ctx context.Context, status string) ([]models.WithdrawalProven, error) {
	withdrawals, err := db.GetWithdrawalsByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawals by status: %w", err)
	}

	provenWithdrawals := make([]models.WithdrawalProven, len(withdrawals))
	for i, withdrawal := range withdrawals {
		provenWithdrawals[i] = models.WithdrawalProven{
			WithdrawalHash: withdrawal.WithdrawalHash,
		}
	}

	return provenWithdrawals, nil
}

// GetWithdrawalProvenByHash gets a withdrawal proven record by its withdrawal hash
func (db *Database) GetWithdrawalProvenByHash(ctx context.Context, withdrawalHash string) (models.WithdrawalProven, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals_proven")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}

	var provenWithdrawal models.WithdrawalProven
	if err := collection.FindOne(ctx, filter).Decode(&provenWithdrawal); err != nil {
		return models.WithdrawalProven{}, fmt.Errorf("failed to get withdrawal proven by hash: %w", err)
	}

	return provenWithdrawal, nil
}

// GetWithdrawalFinalizedByHash gets a withdrawal finalized record by its withdrawal hash
func (db *Database) GetWithdrawalFinalizedByHash(ctx context.Context, withdrawalHash string) (models.WithdrawalFinalized, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals_finalized")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}

	var finalizedWithdrawal models.WithdrawalFinalized
	if err := collection.FindOne(ctx, filter).Decode(&finalizedWithdrawal); err != nil {
		return models.WithdrawalFinalized{}, fmt.Errorf("failed to get withdrawal finalized by hash: %w", err)
	}

	return finalizedWithdrawal, nil
}

func (db *Database) UpdateWithdrawalStatusByWithdrawalHash(ctx context.Context, withdrawalHash string, status string) error {
	collection := db.client.Database(db.databaseName).Collection("withdrawals")

	filter := bson.D{{Key: "withdrawal_hash", Value: withdrawalHash}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: status}}}}

	_, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	return nil
}
