package lightlink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	L2CrossDomainMessengerContract "github.com/lightlink-network/ll-bridge-api/contracts/L2CrossDomainMessenger"
	L2StandardBridgeContract "github.com/lightlink-network/ll-bridge-api/contracts/L2StandardBridge"
	"github.com/lightlink-network/ll-bridge-api/contracts/L2ToL1MessagePasser"
	"github.com/lightlink-network/ll-bridge-api/utils"
)

type Client struct {
	client                 *ethclient.Client
	chainId                *big.Int
	l2StandardBridge       *L2StandardBridgeContract.L2StandardBridge
	l2CrossDomainMessenger *L2CrossDomainMessengerContract.L2CrossDomainMessenger
	l2ToL1MessagePasser    *L2ToL1MessagePasser.L2ToL1MessagePasser
	logger                 *slog.Logger
	Opts                   *ClientOpts
}

type ClientOpts struct {
	Endpoint                      string
	L2StandardBridgeAddress       common.Address
	L2CrossDomainMessengerAddress common.Address
	L2ToL1MessagePasserAddress    common.Address
	Logger                        *slog.Logger
	Timeout                       time.Duration
	DefaultStartBlock             uint64
}

// NewEthereumRPC returns a new EthereumRPC client over HTTP.
func NewClient(opts ClientOpts) (*Client, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	client, err := ethclient.Dial(opts.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LightLink: %w", err)
	}

	l2StandardBridge, err := L2StandardBridgeContract.NewL2StandardBridge(opts.L2StandardBridgeAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L2StandardBridge: %w", err)
	}

	l2CrossDomainMessenger, err := L2CrossDomainMessengerContract.NewL2CrossDomainMessenger(opts.L2CrossDomainMessengerAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L2CrossDomainMessenger: %w", err)
	}

	l2ToL1MessagePasser, err := L2ToL1MessagePasser.NewL2ToL1MessagePasser(opts.L2ToL1MessagePasserAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L2ToL1MessagePasser: %w", err)
	}

	chainId, err := client.ChainID(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get chainId: %w", err)
	}

	opts.Logger.Info("Connected to LightLink", "chainId", chainId)

	// Warn user if the contracts are not found at the given addresses.
	if ok, _ := utils.IsContract(client, opts.L2StandardBridgeAddress); !ok {
		opts.Logger.Warn("contract not found for L2StandardBridge at given Address", "address", opts.L2StandardBridgeAddress.Hex(), "endpoint", opts.Endpoint)
	}
	if ok, _ := utils.IsContract(client, opts.L2CrossDomainMessengerAddress); !ok {
		opts.Logger.Warn("contract not found for L2CrossDomainMessenger at given Address", "address", opts.L2CrossDomainMessengerAddress.Hex(), "endpoint", opts.Endpoint)
	}
	if ok, _ := utils.IsContract(client, opts.L2ToL1MessagePasserAddress); !ok {
		opts.Logger.Warn("contract not found for L2ToL1MessagePasser at given Address", "address", opts.L2ToL1MessagePasserAddress.Hex(), "endpoint", opts.Endpoint)
	}

	return &Client{
		client:                 client,
		chainId:                chainId,
		l2StandardBridge:       l2StandardBridge,
		l2CrossDomainMessenger: l2CrossDomainMessenger,
		l2ToL1MessagePasser:    l2ToL1MessagePasser,
		logger:                 opts.Logger,
		Opts:                   &opts,
	}, nil
}

// GetBlockTimestamp gets the timestamp of a block by its number using a direct RPC call
func (c *Client) GetBlockTimestamp(blockNumber uint64) (uint64, error) {
	// Convert block number to hex
	blockNumberHex := fmt.Sprintf("0x%x", blockNumber)

	// Construct the JSON-RPC request body
	requestBody := fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"method": "eth_getBlockByNumber",
		"params": ["%s", false],
		"id": 1
	}`, blockNumberHex)

	// Create the request
	req, err := http.NewRequest("POST", c.Opts.Endpoint, strings.NewReader(requestBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Make the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response
	var result struct {
		Result struct {
			Timestamp string `json:"timestamp"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert hex timestamp to uint64
	timestampHex := strings.TrimPrefix(result.Result.Timestamp, "0x")
	timestamp, err := strconv.ParseUint(timestampHex, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return timestamp, nil
}
