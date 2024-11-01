package api

import (
	"net/http"
	"strconv"

	"github.com/lightlink-network/ll-bridge-api/database/models"
)

func (s *Server) handleTransactionsGet(w http.ResponseWriter, r *http.Request) {
	// Get query parameters
	page, err := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.ParseInt(r.URL.Query().Get("pageSize"), 10, 64)
	if err != nil || pageSize < 1 {
		pageSize = 10
	}

	// Build filter from query parameters
	filter := models.Filter{
		Status: r.URL.Query().Get("status"),
		From:   r.URL.Query().Get("from"),
		To:     r.URL.Query().Get("to"),
		TxHash: r.URL.Query().Get("txHash"),
		Type:   r.URL.Query().Get("type"),
	}

	// Get transactions
	result, err := s.db.GetTransactions(r.Context(), filter, page, pageSize)
	if err != nil {
		ERROR(w, http.StatusInternalServerError, err)
		return
	}

	JSON(w, http.StatusOK, result)
}
