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

func NewDatabase(opts DatabaseOpts) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
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

func (db *Database) GetDeposits(ctx context.Context, filter models.Filter, page, pageSize int64) (*models.PaginatedResult, error) {
	collection := db.client.Database(db.databaseName).Collection("deposits")

	mongoFilter := buildFilter(filter)

	// Calculate skip
	skip := (page - 1) * pageSize

	// Get total count
	total, err := collection.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to count deposits: %w", err)
	}

	// Get paginated results
	opts := options.Find().
		SetSort(bson.D{{Key: "block_number", Value: -1}}).
		SetSkip(skip).
		SetLimit(pageSize)

	cursor, err := collection.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find deposits: %w", err)
	}
	defer cursor.Close(ctx)

	var deposits []models.Deposit
	if err := cursor.All(ctx, &deposits); err != nil {
		return nil, fmt.Errorf("failed to decode deposits: %w", err)
	}

	return &models.PaginatedResult{
		Items:      deposits,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func (db *Database) GetWithdrawals(ctx context.Context, filter models.Filter, page, pageSize int64) (*models.PaginatedResult, error) {
	collection := db.client.Database(db.databaseName).Collection("withdrawals")

	mongoFilter := buildFilter(filter)
	skip := (page - 1) * pageSize

	pipeline := mongo.Pipeline{
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
		{{Key: "$sort", Value: bson.D{{Key: "block_number", Value: -1}}}},
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: pageSize}},
	}

	// Get total count (without pagination)
	countPipeline := mongo.Pipeline{
		{{Key: "$match", Value: mongoFilter}},
		{{Key: "$count", Value: "count"}},
	}

	var countResult []bson.M
	countCursor, err := collection.Aggregate(ctx, countPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to count withdrawals: %w", err)
	}
	defer countCursor.Close(ctx)

	if err := countCursor.All(ctx, &countResult); err != nil {
		return nil, fmt.Errorf("failed to decode count result: %w", err)
	}

	total := int64(0)
	if len(countResult) > 0 {
		total = int64(countResult[0]["count"].(int32))
	}

	// Execute main pipeline
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to execute withdrawal aggregation: %w", err)
	}
	defer cursor.Close(ctx)

	var withdrawals []models.Withdrawal
	if err := cursor.All(ctx, &withdrawals); err != nil {
		return nil, fmt.Errorf("failed to decode withdrawals: %w", err)
	}

	return &models.PaginatedResult{
		Items:      withdrawals,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
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

func (db *Database) GetTransactions(ctx context.Context, filter models.Filter, page, pageSize int64) (*models.PaginatedResult, error) {
	// Get deposits
	depositResults, err := db.GetDeposits(ctx, filter, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get deposits: %w", err)
	}

	// Get withdrawals with proven/finalized status
	withdrawalResults, err := db.GetWithdrawals(ctx, filter, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawals: %w", err)
	}

	// Combine results
	transactions := make([]interface{}, 0)

	// Add deposits
	deposits := depositResults.Items.([]models.Deposit)
	for _, d := range deposits {
		transactions = append(transactions, models.Transaction{
			Type:        "deposit",
			ERC20:       d.ERC20,
			From:        d.From,
			To:          d.To,
			Value:       d.Value,
			L1Token:     d.L1Token,
			L2Token:     d.L2Token,
			Message:     d.Message,
			MessageHash: d.MessageHash,
			TxHash:      d.TxHash,
			L1TxHash:    d.L1TxHash,
			BlockNumber: d.BlockNumber,
			BlockHash:   d.BlockHash,
			BlockTime:   d.BlockTime,
			Status:      d.Status,
		})
	}

	// Add withdrawals
	withdrawals := withdrawalResults.Items.([]models.Withdrawal)
	for _, w := range withdrawals {
		// Determine status - if there's a finalized tx hash, set status to "RELAYED"
		status := w.Status
		// if w.FinalizeTx.TxHash != "" {
		// 	status = "RELAYED"
		// }

		transactions = append(transactions, models.Transaction{
			Type:           "withdrawal",
			ERC20:          w.ERC20,
			From:           w.From,
			To:             w.To,
			Value:          w.Value,
			L1Token:        w.L1Token,
			L2Token:        w.L2Token,
			Message:        w.Message,
			MessageHash:    w.MessageHash,
			WithdrawalHash: w.WithdrawalHash,
			TxHash:         w.TxHash,
			BlockNumber:    w.BlockNumber,
			BlockHash:      w.BlockHash,
			BlockTime:      w.BlockTime,
			Status:         status, // Use the updated status
			ProvenTx:       w.ProveTx,
			FinalizeTx:     w.FinalizeTx,
		})
	}

	// Calculate total count
	totalCount := depositResults.TotalCount + withdrawalResults.TotalCount

	return &models.PaginatedResult{
		Items:      transactions,
		TotalCount: totalCount,
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

	filter := bson.D{{Key: "status", Value: status}}

	cursor, err := collection.Find(ctx, filter)
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
