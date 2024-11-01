package ethereum

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	CanonicalStateChainContract "github.com/lightlink-network/ll-bridge-api/contracts/CanonicalStateChain"
	L1StandardBridgeContract "github.com/lightlink-network/ll-bridge-api/contracts/L1StandardBridge"
	LightLinkPortalContract "github.com/lightlink-network/ll-bridge-api/contracts/LightLinkPortal"
	"github.com/lightlink-network/ll-bridge-api/utils"
)

type Client struct {
	client              *ethclient.Client
	chainId             *big.Int
	l1StandardBridge    *L1StandardBridgeContract.L1StandardBridge
	lightlinkPortal     *LightLinkPortalContract.LightLinkPortal
	canonicalStateChain *CanonicalStateChainContract.CanonicalStateChain
	logger              *slog.Logger
	Opts                *ClientOpts
}

type ClientOpts struct {
	Endpoint                      string
	L1StandardBridgeAddress       common.Address
	L1CrossDomainMessengerAddress common.Address
	LightLinkPortalAddress        common.Address
	CanonicalStateChainAddress    common.Address
	Logger                        *slog.Logger
	Timeout                       time.Duration
	DefaultStartBlock             uint64
	StatusCheckInterval           uint64
}

// NewEthereumRPC returns a new EthereumRPC client over HTTP.
func NewClient(opts ClientOpts) (*Client, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	client, err := ethclient.Dial(opts.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum: %w", err)
	}

	l1StandardBridge, err := L1StandardBridgeContract.NewL1StandardBridge(opts.L1StandardBridgeAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L1StandardBridge: %w", err)
	}

	lightlinkPortal, err := LightLinkPortalContract.NewLightLinkPortal(opts.LightLinkPortalAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LightLinkPortal: %w", err)
	}

	canonicalStateChain, err := CanonicalStateChainContract.NewCanonicalStateChain(opts.CanonicalStateChainAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CanonicalStateChain: %w", err)
	}

	chainId, err := client.ChainID(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get chainId: %w", err)
	}

	opts.Logger.Info("Connected to Ethereum", "chainId", chainId)

	// Warn user if the contracts are not found at the given addresses.
	if ok, _ := utils.IsContract(client, opts.CanonicalStateChainAddress); !ok {
		opts.Logger.Warn("contract not found for CanonicalStateChain at given Address", "address", opts.CanonicalStateChainAddress.Hex(), "endpoint", opts.Endpoint)
	}
	if ok, _ := utils.IsContract(client, opts.L1StandardBridgeAddress); !ok {
		opts.Logger.Warn("contract not found for L1StandardBridge at given Address", "address", opts.L1StandardBridgeAddress.Hex(), "endpoint", opts.Endpoint)
	}
	if ok, _ := utils.IsContract(client, opts.LightLinkPortalAddress); !ok {
		opts.Logger.Warn("contract not found for LightLinkPortal at given Address", "address", opts.LightLinkPortalAddress.Hex(), "endpoint", opts.Endpoint)
	}

	return &Client{
		client:              client,
		chainId:             chainId,
		l1StandardBridge:    l1StandardBridge,
		lightlinkPortal:     lightlinkPortal,
		canonicalStateChain: canonicalStateChain,
		logger:              opts.Logger,
		Opts:                &opts,
	}, nil
}
