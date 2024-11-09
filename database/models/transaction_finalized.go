package models

// TransactionFinalized represents a finalized transaction. It is
// stored in a separate collection as it may be indexed before
// init tx.
type TransactionFinalized struct {
	WithdrawalHash string `json:"withdrawal_hash" bson:"withdrawal_hash"`
	TxHash         string `json:"tx_hash" bson:"tx_hash"`
	BlockNumber    uint64 `json:"block_number" bson:"block_number"`
	Timestamp      uint64 `json:"timestamp" bson:"timestamp"`
	GasUsed        uint64 `json:"gas_used" bson:"gas_used"`
}
