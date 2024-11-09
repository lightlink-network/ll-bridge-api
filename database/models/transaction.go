package models

import "time"

// Transaction represents either a deposit or withdrawal init tx with all related information.
// We store deposits and withdrawals in the same collection to simplify querying since the frontend
// preference is to query them together.
type Transaction struct {
	Type   string `json:"type" bson:"type"` // "deposit" or "withdrawal"
	ERC20  bool   `json:"erc20" bson:"erc20"`
	Status string `json:"status" bson:"status"`

	// Init tx metadata
	TxHash      string `json:"tx_hash" bson:"tx_hash"`
	BlockNumber uint64 `json:"block_number" bson:"block_number"`
	BlockHash   string `json:"block_hash" bson:"block_hash"`
	BlockTime   uint64 `json:"block_time" bson:"block_time"`
	GasUsed     uint64 `json:"gas_used" bson:"gas_used"`

	// Init tx
	From        string `json:"from" bson:"from"`
	To          string `json:"to" bson:"to"`
	Value       string `json:"value" bson:"value"`
	L1Token     string `json:"l1_token,omitempty" bson:"l1_token,omitempty"`
	L2Token     string `json:"l2_token,omitempty" bson:"l2_token,omitempty"`
	Message     string `json:"message,omitempty" bson:"message,omitempty"`
	MessageHash string `json:"message_hash,omitempty" bson:"message_hash,omitempty"`

	// Deposits only
	L2TxHash string `json:"l2_tx_hash,omitempty" bson:"l2_tx_hash,omitempty"`

	// Withdrawals only
	WithdrawalHash string                `json:"withdrawal_hash,omitempty" bson:"withdrawal_hash,omitempty"`
	ProvenTx       *TransactionProven    `json:"prove_tx,omitempty" bson:"prove_tx,omitempty"`
	FinalizeTx     *TransactionFinalized `json:"finalize_tx,omitempty" bson:"finalize_tx,omitempty"`

	// DB Timestamps
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}
