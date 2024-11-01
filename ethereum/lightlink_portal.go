package ethereum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	LightLinkPortalContract "github.com/lightlink-network/ll-bridge-api/contracts/LightLinkPortal"
)

type LightLinkPortal interface {
	FilterWithdrawalProven(opts *bind.FilterOpts, withdrawalHash [][32]byte, from []common.Address, to []common.Address) (*LightLinkPortalContract.LightLinkPortalWithdrawalProvenIterator, error)
	FilterWithdrawalFinalized(opts *bind.FilterOpts, withdrawalHash [][32]byte) (*LightLinkPortalContract.LightLinkPortalWithdrawalFinalizedIterator, error)
	ProvenWithdrawals(opts *bind.CallOpts, arg0 [32]byte) (struct {
		OutputRoot    [32]byte
		Timestamp     *big.Int
		L2OutputIndex *big.Int
	}, error)
	IsOutputFinalized(opts *bind.CallOpts, l2OutputIndex *big.Int) (bool, error)
}

var _ LightLinkPortal = &Client{}

func (c *Client) FilterWithdrawalProven(opts *bind.FilterOpts, withdrawalHash [][32]byte, from []common.Address, to []common.Address) (*LightLinkPortalContract.LightLinkPortalWithdrawalProvenIterator, error) {
	return c.lightlinkPortal.FilterWithdrawalProven(opts, withdrawalHash, from, to)
}

func (c *Client) FilterWithdrawalFinalized(opts *bind.FilterOpts, withdrawalHash [][32]byte) (*LightLinkPortalContract.LightLinkPortalWithdrawalFinalizedIterator, error) {
	return c.lightlinkPortal.FilterWithdrawalFinalized(opts, withdrawalHash)
}

func (c *Client) ProvenWithdrawals(opts *bind.CallOpts, arg0 [32]byte) (struct {
	OutputRoot    [32]byte
	Timestamp     *big.Int
	L2OutputIndex *big.Int
}, error) {
	return c.lightlinkPortal.ProvenWithdrawals(opts, arg0)
}

func (c *Client) IsOutputFinalized(opts *bind.CallOpts, l2OutputIndex *big.Int) (bool, error) {
	return c.lightlinkPortal.IsOutputFinalized(opts, l2OutputIndex)
}
