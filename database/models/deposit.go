package models

type Deposit struct {
	Type        string `json:"type" bson:"type"`
	ERC20       bool   `json:"erc20" bson:"erc20"`
	From        string `json:"from" bson:"from"`
	To          string `json:"to" bson:"to"`
	Value       string `json:"value" bson:"value"`
	L1Token     string `json:"l1_token" bson:"l1_token"`
	L2Token     string `json:"l2_token" bson:"l2_token"`
	Message     string `json:"message" bson:"message"`
	MessageHash string `json:"message_hash" bson:"message_hash"`
	TxHash      string `json:"tx_hash" bson:"tx_hash"`
	L1TxHash    string `json:"l1_tx_hash" bson:"l1_tx_hash"`
	BlockNumber uint64 `json:"block_number" bson:"block_number"`
	BlockHash   string `json:"block_hash" bson:"block_hash"`
	BlockTime   uint64 `json:"block_time" bson:"block_time"`
	Status      string `json:"status" bson:"status"`
}
