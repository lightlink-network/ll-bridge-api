package models

type WithdrawalProven struct {
	WithdrawalHash string `json:"withdrawal_hash" bson:"withdrawal_hash"`
	TxHash         string `json:"tx_hash" bson:"tx_hash"`
	BlockNumber    uint64 `json:"block_number" bson:"block_number"`
	Timestamp      uint64 `json:"timestamp" bson:"timestamp"`
	L2OutputIndex  uint64 `json:"l2_output_index" bson:"l2_output_index"`
}
