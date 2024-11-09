package models

// LastIndexedBlock represents the last indexed block for a given chain.
// It is used to track the last block indexed for a given chain to avoid
// reprocessing the same block when the API is restarted.
type LastIndexedBlock struct {
	Chain       string `json:"chain" bson:"chain"`
	BlockNumber uint64 `json:"block_number" bson:"block_number"`
}
