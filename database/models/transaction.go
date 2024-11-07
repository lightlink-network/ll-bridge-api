package models

// Transaction represents either a deposit or withdrawal with all related information
type Transaction struct {
	Type           string               `json:"type" bson:"type"` // "deposit" or "withdrawal"
	ERC20          bool                 `json:"erc20" bson:"erc20"`
	From           string               `json:"from" bson:"from"`
	To             string               `json:"to" bson:"to"`
	Value          string               `json:"value" bson:"value"`
	L1Token        string               `json:"l1_token,omitempty" bson:"l1_token,omitempty"`
	L2Token        string               `json:"l2_token,omitempty" bson:"l2_token,omitempty"`
	Message        string               `json:"message,omitempty" bson:"message,omitempty"`
	MessageHash    string               `json:"message_hash,omitempty" bson:"message_hash,omitempty"`
	WithdrawalHash string               `json:"withdrawal_hash,omitempty" bson:"withdrawal_hash,omitempty"`
	TxHash         string               `json:"tx_hash" bson:"tx_hash"`
	L1TxHash       string               `json:"l2_tx_hash,omitempty" bson:"l1_tx_hash,omitempty"`
	BlockNumber    uint64               `json:"block_number" bson:"block_number"`
	BlockHash      string               `json:"block_hash" bson:"block_hash"`
	BlockTime      uint64               `json:"block_time" bson:"block_time"`
	Status         string               `json:"status" bson:"status"`
	ProvenTx       *WithdrawalProven    `json:"prove_tx,omitempty" bson:"prove_tx,omitempty"`
	FinalizeTx     *WithdrawalFinalized `json:"finalize_tx,omitempty" bson:"finalize_tx,omitempty"`
}
