package lightlink

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	L2CrossDomainMessengerContract "github.com/lightlink-network/ll-bridge-api/contracts/L2CrossDomainMessenger"
	L2StandardBridgeContract "github.com/lightlink-network/ll-bridge-api/contracts/L2StandardBridge"
)

type L2StandardBridge interface {
	FilterETHBridgeInitiated(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*L2StandardBridgeContract.L2StandardBridgeETHBridgeInitiatedIterator, error)
	FilterERC20BridgeInitiated(opts *bind.FilterOpts, localToken []common.Address, remoteToken []common.Address, from []common.Address) (*L2StandardBridgeContract.L2StandardBridgeERC20BridgeInitiatedIterator, error)
	FilterRelayedMessage(opts *bind.FilterOpts, msgHash [][32]byte) (*L2CrossDomainMessengerContract.L2CrossDomainMessengerRelayedMessageIterator, error)
	BlockNumber() (uint64, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
}

var _ L2StandardBridge = &Client{}

func (c *Client) FilterETHBridgeInitiated(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*L2StandardBridgeContract.L2StandardBridgeETHBridgeInitiatedIterator, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if itr, err := c.l2StandardBridge.FilterETHBridgeInitiated(opts, from, to); err == nil {
			return itr, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return nil, fmt.Errorf("failed to filter ETHBridgeInitiated after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) FilterERC20BridgeInitiated(opts *bind.FilterOpts, localToken []common.Address, remoteToken []common.Address, from []common.Address) (*L2StandardBridgeContract.L2StandardBridgeERC20BridgeInitiatedIterator, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if itr, err := c.l2StandardBridge.FilterERC20BridgeInitiated(opts, localToken, remoteToken, from); err == nil {
			return itr, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return nil, fmt.Errorf("failed to filter ERC20BridgeInitiated after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) BlockNumber() (uint64, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if num, err := c.client.BlockNumber(context.Background()); err == nil {
			return num, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return 0, fmt.Errorf("failed to get block number after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) FilterRelayedMessage(opts *bind.FilterOpts, msgHash [][32]byte) (*L2CrossDomainMessengerContract.L2CrossDomainMessengerRelayedMessageIterator, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if itr, err := c.l2CrossDomainMessenger.FilterRelayedMessage(opts, msgHash); err == nil {
			return itr, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return nil, fmt.Errorf("failed to filter RelayedMessage after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if receipt, err := c.client.TransactionReceipt(ctx, txHash); err == nil {
			return receipt, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return nil, fmt.Errorf("failed to get transaction receipt after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	maxRetries := 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if block, err := c.client.BlockByNumber(ctx, number); err == nil {
			return block, nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			time.Sleep(time.Second * 2)
		}
	}

	return nil, fmt.Errorf("failed to get block by number after %d attempts: %w", maxRetries, lastErr)
}
