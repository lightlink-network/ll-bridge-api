package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
	"github.com/lightlink-network/ll-bridge-api/api"
	"github.com/lightlink-network/ll-bridge-api/database"
	"github.com/lightlink-network/ll-bridge-api/ethereum"
	"github.com/lightlink-network/ll-bridge-api/indexer"
	"github.com/lightlink-network/ll-bridge-api/lightlink"
	"github.com/lmittmann/tint"
)

// Version will be set at build time
var Version = "development"

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// create a new logger
	Logger := slog.New(tint.NewHandler(os.Stderr, nil))

	// set global logger with custom options
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stderr, &tint.Options{
			Level: slog.LevelDebug,
		}),
	))

	Logger.Info("Starting ll-bridge-api ("+Version+")",
		"Go Version", runtime.Version(),
		"Operating System", runtime.GOOS,
		"Architecture", runtime.GOARCH)

	llDefaultStartBlock, err := strconv.ParseUint(os.Getenv("LL_DEFAULT_START_BLOCK"), 10, 64)
	if err != nil {
		log.Fatalf("failed to parse LL_DEFAULT_START_BLOCK: %v", err)
	}

	ethDefaultStartBlock, err := strconv.ParseUint(os.Getenv("ETH_DEFAULT_START_BLOCK"), 10, 64)
	if err != nil {
		log.Fatalf("failed to parse ETH_DEFAULT_START_BLOCK: %v", err)
	}

	ethStatusCheckInterval, err := strconv.ParseUint(os.Getenv("ETH_STATUS_CHECK_INTERVAL"), 10, 64)
	if err != nil {
		log.Fatalf("failed to parse ETH_STATUS_CHECK_INTERVAL: %v", err)
	}

	indexer, err := indexer.NewIndexer(indexer.IndexerOpts{
		Lightlink: &lightlink.ClientOpts{
			Endpoint:                      os.Getenv("LL_RPC_URL"),
			L2StandardBridgeAddress:       common.HexToAddress(os.Getenv("L2_STANDARD_BRIDGE_ADDRESS")),
			L2CrossDomainMessengerAddress: common.HexToAddress(os.Getenv("L2_CROSS_DOMAIN_MESSENGER_ADDRESS")),
			L2ToL1MessagePasserAddress:    common.HexToAddress(os.Getenv("L2_TO_L1_MESSAGE_PASSER_ADDRESS")),
			Logger:                        Logger.With("component", "lightlink-indexer"),
			DefaultStartBlock:             llDefaultStartBlock,
		},
		Ethereum: &ethereum.ClientOpts{
			Endpoint:                      os.Getenv("ETHEREUM_RPC_URL"),
			L1StandardBridgeAddress:       common.HexToAddress(os.Getenv("L1_STANDARD_BRIDGE_ADDRESS")),
			L1CrossDomainMessengerAddress: common.HexToAddress(os.Getenv("L1_CROSS_DOMAIN_MESSENGER_ADDRESS")),
			LightLinkPortalAddress:        common.HexToAddress(os.Getenv("LIGHTLINK_PORTAL_ADDRESS")),
			CanonicalStateChainAddress:    common.HexToAddress(os.Getenv("CANONICAL_STATE_CHAIN_ADDRESS")),
			Logger:                        Logger.With("component", "ethereum-indexer"),
			DefaultStartBlock:             ethDefaultStartBlock,
			StatusCheckInterval:           ethStatusCheckInterval,
		},
		Database: &database.DatabaseOpts{
			URI:          os.Getenv("DATABASE_URI"),
			DatabaseName: os.Getenv("DATABASE_NAME"),
			Logger:       Logger.With("component", "database"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// start api server
	server, err := api.NewServer(api.ServerOpts{
		Logger:       Logger.With("component", "api-server"),
		URI:          os.Getenv("DATABASE_URI"),
		DatabaseName: os.Getenv("DATABASE_NAME"),
		Port:         os.Getenv("API_PORT"),
	})
	if err != nil {
		log.Fatalf("failed to create api server: %v", err)
	}

	go server.StartServer()

	// Create context that will be canceled on SIGINT or SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start indexer in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- indexer.Run(ctx)
	}()

	// Wait for either error or signal
	select {
	case err := <-errChan:
		if err != nil {
			log.Printf("Indexer error: %v", err)
		}
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal: %v\n", sig)
		fmt.Println("Shutting down gracefully...")
		cancel() // This will trigger shutdown via context

		// Wait for indexer to finish
		if err := <-errChan; err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}
}
