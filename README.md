# ll-bridge-api

Indexer & API for the LightLink standard bridge.

Indexes deposits & withdrawals and automatically monitors and updates their status.

## API

A HTTP API is available at `/v1/`

### Transactions

It returns a list of transactions that match the query.

Query parameters:

- `type` - filter by type (withdrawal or deposit)
- `status` - filter by status
- `from` - filter by from address
- `to` - filter by to address
- `page` - pagination
- `page_size` - pagination

i.e. `GET /v1/transactions?type=withdrawal&status=READY_TO_PROVE&page=1&page_size=10`

## Withdrawal Status Flow

    1. STATE_ROOT_NOT_PUBLISHED (New Withdrawal submitted on L2)
    2. READY_TO_PROVE (After 6-12 hours, user can prove withdrawal on L1)
    3. IN_CHALLENGE_PERIOD (Withdrawal has been proven on L1. Challenge period started on L1, wait 7 days)
    4. READY_FOR_RELAY (Challenge period ended on L1, user can now relay withdrawal on L1)
    5. RELAYED (Message sent to L2)

## Deposit Status Flow

    1. READY_FOR_RELAY (New Deposit submitted on L1)
    2. RELAYED (Deposit complete on L2 automatically by Sequencer)
