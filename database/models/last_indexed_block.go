package models

type LastIndexedBlock struct {
	Chain       string `json:"chain" bson:"chain"`
	BlockNumber uint64 `json:"block_number" bson:"block_number"`
}
