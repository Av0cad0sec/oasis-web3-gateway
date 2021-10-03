package indexer

import (
	"context"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/runtime/client/api"
	"github.com/oasisprotocol/oasis-core/go/runtime/transaction"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	localStorage "github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

// Result is a query result.
type Result struct {
	// TxHash is the hash of the matched transaction.
	TxHash hash.Hash
	// TxIndex is the index of the matched transaction within the block.
	TxIndex uint32
}

// Results are query results.
//
// Map key is the round number and value is a list of transaction hashes
// that match the query.
type Results map[uint64][]Result

// BackendFactory is the indexer backend factory interface.
type BackendFactory func(
	dataDir string, 
	runtimeID common.Namespace, 
	storage localStorage.storage) (Backend, error)

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// querie block round by block hash.
	QueryBlockRound(blockHash hash.Hash) (uint64, error)

	QueryBlockHash(round uint64) (hash.Hash, error)

	// querie oasis tx result by ethereum tx hash.
	QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error)
}

// Backend is the indexer backend interface.
type Backend interface {
	QueryableBackend

	Index(
		ctx context.Context,
		round uint64,
		blockHash hash.Hash,
		txs []*transaction.Transaction,
		tags transaction.Tags,
	) error

	Close()
}

type psqlBackend struct {
	logger *logging.Logger
	storage localStorage.Storage
}

func (p *psqlBackend) Index(
	ctx context.Context,
	round uint64,
	blockHash hash.Hash,
	txs []*transaction.Transaction,
	tags transaction.Tags,
) error {
	//block round <-> block hash
	blockRef := &model.Block{
		Round: round,
		Hash:  blockHash,
	}

	p.storage.Store(blockRef)

	//eth tx hash <-> oasis tx result
	for idx, tx := range txs {
		ethTxHash = "decode eth tx from oasis tx, and get eth tx hash"
		txRef := &model.Transaction{
			EthTx: ethTxHash,
			Result: &model.TxResult{
				Hash:  tx.Hash(),
				Index: uint32(idx),
				Round: round,
			},
		}

		p.storage.Store(txRef)
	}

	//tags?

	return nil
}

func (p *psqlBackend)QueryBlockRound(blockHash hash.Hash) (uint64, error) {
	round, err := p.storage.GetBlockRound(blockHash)

	if err != nil {
		p.logger.Error("Can't find matched block")
		return 0, err 
	}

	return round, nil
}


func (p *psqlBackend)QueryBlockHash(round uint64) (hash.Hash, error) {
	hash, err := p.storage.GetBlockHash(round)

	if err != nil {
		panic("Indexer error!")
		return 0, err 
	}

	return hash, nil
}

func (p *psqlBackend)QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error) {
	result, err = p.storage.GetTxResult(ethTransactionHash)

	if err != nil {
		p.logger.Error("Can't find matched transaction result")
		return nil, err
	}

	return result, nil
}

func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

func newPsqlBackend(storage localStorage.Storage) (Backend, error) {
	b := &psqlBackend{
		logger:   logging.GetLogger("gateway/indexer/backend").With("runtime_id", runtimeID),
		storage: storage
	}

	b.logger.info("New psql backend")

	return b, nil
}

func NewPsqlBackend(storage localStorage.Storage) BackendFactory {
	return newPsqlBackend 
}
