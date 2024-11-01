package models

type WithdrawalFinalized struct {
	WithdrawalHash string `json:"withdrawal_hash" bson:"withdrawal_hash"`
	TxHash         string `json:"tx_hash" bson:"tx_hash"`
	BlockNumber    uint64 `json:"block_number" bson:"block_number"`
	Timestamp      uint64 `json:"timestamp" bson:"timestamp"`
}
