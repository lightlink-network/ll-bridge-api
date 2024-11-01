package ethereum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	CanonicalStateChainContract "github.com/lightlink-network/ll-bridge-api/contracts/CanonicalStateChain"
)

type CanonicalStateChain interface {
	FilterOutputProposed(opts *bind.FilterOpts, outputRoot [][32]byte, l2OutputIndex []*big.Int, l2BlockNumber []*big.Int) (*CanonicalStateChainContract.CanonicalStateChainOutputProposedIterator, error)
}

var _ CanonicalStateChain = &Client{}

func (c *Client) FilterOutputProposed(opts *bind.FilterOpts, outputRoot [][32]byte, l2OutputIndex []*big.Int, l2BlockNumber []*big.Int) (*CanonicalStateChainContract.CanonicalStateChainOutputProposedIterator, error) {
	return c.canonicalStateChain.FilterOutputProposed(opts, outputRoot, l2OutputIndex, l2BlockNumber)
}
