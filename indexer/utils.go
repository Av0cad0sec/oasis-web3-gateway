package indexer

import (
	"encoding/hex"
	"math/big"

	"github.com/fxamacker/cbor/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

var (
	defaultValidatorAddr = "0x0000000000000000000000000000000088888888"
	defaultGasLimit      = 21000 * 1000
	EmptyRootHash        = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

// Log is the Oasis Log.
type Log struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}

func Logs2EthLogs(logs []*Log, round uint64, blockHash, txHash common.Hash, txIndex uint32) []*ethtypes.Log {
	ethLogs := []*ethtypes.Log{}
	for i := range logs {
		ethLog := &ethtypes.Log{
			Address:     logs[i].Address,
			Topics:      logs[i].Topics,
			Data:        logs[i].Data,
			BlockNumber: round,
			TxHash:      txHash,
			TxIndex:     uint(txIndex),
			BlockHash:   blockHash,
			Index:       uint(i),
			Removed:     false,
		}
		ethLogs = append(ethLogs, ethLog)
	}
	return ethLogs
}

func convertToEthFormat(
	block *block.Block,
	transactions ethtypes.Transactions,
	logs []*ethtypes.Log,
	txsStatus []uint8,
	results []types.CallResult,
	gas uint64,
) (*model.Block, []*model.Transaction, []map[string]interface{}, error) {
	encoded := block.Header.EncodedHash()
	bhash, _ := encoded.MarshalBinary()
	bprehash, _ := block.Header.PreviousHash.MarshalBinary()
	bshash, _ := block.Header.StateRoot.MarshalBinary()
	bloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))
	btxHash, _ := block.Header.IORoot.MarshalBinary()

	baseFee := big.NewInt(10)
	number := big.NewInt(0)
	number.SetUint64(block.Header.Round)
	bloomData, _ := bloom.MarshalText()

	innerHeader := &model.Header{
		ParentHash:  common.BytesToHash(bprehash).Hex(),
		UncleHash:   ethtypes.EmptyUncleHash.Hex(),
		Coinbase:    common.HexToAddress(defaultValidatorAddr).Hex(),
		Root:        common.BytesToHash(bshash).Hex(),
		TxHash:      string(btxHash),
		ReceiptHash: ethtypes.EmptyRootHash.Hex(),
		Bloom:       string(bloomData),
		Difficulty:  big.NewInt(0).String(),
		Number:      number.String(),
		GasLimit:    uint64(defaultGasLimit),
		GasUsed:     gas,
		Time:        uint64(block.Header.Timestamp),
		Extra:       "",
		MixDigest:   "",
		Nonce:       ethtypes.BlockNonce{}.Uint64(),
		BaseFee:     baseFee.String(),
	}

	innerTxs := []*model.Transaction{}
	innerReceipts := []map[string]interface{}{}
	cumulativeGasUsed := uint64(0)
	for idx, ethTx := range transactions {
		r, s, v := ethTx.RawSignatureValues()
		signer := ethtypes.LatestSignerForChainID(ethTx.ChainId())
		from, _ := signer.Sender(ethTx)
		ethAccList := ethTx.AccessList()
		accList := []model.AccessTuple{}
		for _, ethAcc := range ethAccList {
			keys := []string{}
			for _, ethStorageKey := range ethAcc.StorageKeys {
				keys = append(keys, ethStorageKey.Hex())
			}
			acc := model.AccessTuple{
				Address:     ethAcc.Address.Hex(),
				StorageKeys: keys,
			}
			accList = append(accList, acc)
		}
		tx := &model.Transaction{
			Hash:       ethTx.Hash().Hex(),
			Type:       ethTx.Type(),
			ChainID:    ethTx.ChainId().String(),
			Status:     uint(txsStatus[idx]),
			BlockHash:  string(bhash),
			Round:      number.Uint64(),
			Index:      uint32(idx),
			Gas:        ethTx.Gas(),
			GasPrice:   ethTx.GasPrice().String(),
			GasTipCap:  ethTx.GasTipCap().String(),
			GasFeeCap:  ethTx.GasFeeCap().String(),
			Nonce:      ethTx.Nonce(),
			FromAddr:   from.Hex(),
			Value:      ethTx.Value().String(),
			Data:       hex.EncodeToString(ethTx.Data()),
			AccessList: accList,
			V:          v.String(),
			R:          r.String(),
			S:          s.String(),
		}
		to := ethTx.To()
		if to == nil {
			tx.ToAddr = ""
		} else {
			tx.ToAddr = to.Hex()
		}
		innerTxs = append(innerTxs, tx)

		// receipts
		cumulativeGasUsed += tx.Gas
		receipt := map[string]interface{}{
			"status":            hexutil.Uint(txsStatus[idx]),
			"cumulativeGasUsed": hexutil.Uint64(cumulativeGasUsed),
			"logsBloom":         ethtypes.BytesToBloom(ethtypes.LogsBloom(logs)),
			"logs":              logs,
			"transactionHash":   ethTx.Hash().Hex(),
			"gasUsed":           hexutil.Uint64(ethTx.Gas()),
			"type":              hexutil.Uint64(ethTx.Type()),
			"blockHash":         bhash,
			"blockNumber":       hexutil.Uint64(block.Header.Round),
			"transactionIndex":  hexutil.Uint64(idx),
			"from":              nil,
			"to":                nil,
			"contractAddress":   nil,
		}
		if tx.FromAddr != "" {
			receipt["from"] = tx.FromAddr
		}
		if tx.ToAddr != "" {
			receipt["to"] = tx.ToAddr
		}
		if logs == nil {
			receipt["logs"] = [][]*ethtypes.Log{}
		}
		if tx.ToAddr == "" && results[idx].IsSuccess() {
			var out []byte
			if err := cbor.Unmarshal(results[idx].Ok, &out); err != nil {
				return nil, nil, nil, err
			}
			receipt["contractAddress"] = common.BytesToAddress(out)
		}
		innerReceipts = append(innerReceipts, receipt)
	}

	innerBlock := &model.Block{
		Hash:         string(bhash),
		Round:        number.Uint64(),
		Header:       innerHeader,
		Uncles:       []*model.Header{},
		Transactions: innerTxs,
	}
	return innerBlock, innerTxs, innerReceipts, nil
}

func (p *psqlBackend) StoreBlockData(oasisBlock *block.Block, txResults []*client.TransactionWithResults) error {
	encoded := oasisBlock.Header.EncodedHash()
	bhash, _ := encoded.MarshalBinary()
	blockNum := oasisBlock.Header.Round

	ethTxs := ethtypes.Transactions{}
	logs := []*ethtypes.Log{}
	txsStatus := []uint8{}
	results := []types.CallResult{}
	var gasUsed uint64

	// Decode tx and get logs
	for txIndex, item := range txResults {
		oasisTx := item.Tx
		if len(oasisTx.AuthProofs) != 1 || oasisTx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}

		// Extract raw Ethereum transaction for further processing.
		rawEthTx := oasisTx.Body
		// Decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		err := rlp.DecodeBytes(rawEthTx, ethTx)
		if err != nil {
			p.logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "error", err.Error())
			continue
		}

		status := uint8(0)
		if txResults[txIndex].Result.IsSuccess() {
			status = uint8(ethtypes.ReceiptStatusSuccessful)
		} else {
			status = uint8(ethtypes.ReceiptStatusFailed)
		}
		txsStatus = append(txsStatus, status)
		results = append(results, item.Result)

		gasUsed += ethTx.Gas()
		ethTxs = append(ethTxs, ethTx)

		var oasisLogs []*Log
		resEvents := item.Events
		for eventIndex, event := range resEvents {
			if event.Code == 1 {
				log := &Log{}
				if err := cbor.Unmarshal(event.Value, log); err != nil {
					p.logger.Error("Failed to unmarshal event value", "index", eventIndex)
					continue
				}
				oasisLogs = append(oasisLogs, log)
			}
		}

		logs = Logs2EthLogs(oasisLogs, oasisBlock.Header.Round, common.BytesToHash(bhash), ethTx.Hash(), uint32(txIndex))
		// store logs
		dbLogs := eth2DbLogs(logs)
		p.storage.Store(dbLogs)
	}

	// Get convert block, transactions and receipts
	blk, txs, receipts, err := convertToEthFormat(oasisBlock, ethTxs, logs, txsStatus, results, gasUsed)
	if err != nil {
		p.logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return err
	}

	// Store txs
	err = p.storage.Store(txs)
	if err != nil {
		return err
	}

	// Store receipts
	err = p.storage.Store(receipts)
	if err != nil {
		return err
	}

	// Store block
	err = p.storage.Store(blk)
	if err != nil {
		return err
	}

	return nil
}

func eth2DbLogs(ethLogs []*ethtypes.Log) []*model.Log {
	res := []*model.Log{}

	for _, log := range ethLogs {
		topics := []string{}
		for _, tp := range log.Topics {
			topics = append(topics, tp.Hex())
		}

		dbLog := &model.Log{
			Address:   log.Address.Hex(),
			Topics:    topics,
			Data:      hex.EncodeToString(log.Data),
			Round:     log.BlockNumber,
			BlockHash: log.BlockHash.Hex(),
			TxHash:    log.TxHash.Hex(),
			TxIndex:   log.TxIndex,
			Index:     log.Index,
			Removed:   log.Removed,
		}

		res = append(res, dbLog)
	}

	return res
}
