package models

type Filter struct {
	Status string
	From   string
	To     string
	TxHash string
	Type   string
}

type PaginatedResult struct {
	Items      interface{} `json:"items"`
	TotalCount int64       `json:"total_count"`
	Page       int64       `json:"page"`
	PageSize   int64       `json:"page_size"`
}