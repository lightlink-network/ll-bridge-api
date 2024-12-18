package types

// MessageStatus represents the different states a cross-chain message can be in
type MessageStatus string

const (

	// UnconfirmedL1ToL2Message - Message is an L1 to L2 message and has not been confirmed
	UnconfirmedL1ToL2Message MessageStatus = "UNCONFIRMED_L1_TO_L2_MESSAGE"

	// StateRootNotPublished - Message is an L2 to L1 message and no state root has been published yet
	StateRootNotPublished MessageStatus = "STATE_ROOT_NOT_PUBLISHED"

	// ReadyToProve - Message is ready to be proved on L1 to initiate the challenge period
	ReadyToProve MessageStatus = "READY_TO_PROVE"

	// InChallengePeriod - Message is a proved L2 to L1 message and is undergoing the challenge period
	InChallengePeriod MessageStatus = "IN_CHALLENGE_PERIOD"

	// ReadyForRelay - Message is ready to be relayed
	ReadyForRelay MessageStatus = "READY_FOR_RELAY"

	// Relayed - Message has been relayed
	Relayed MessageStatus = "RELAYED"
)
