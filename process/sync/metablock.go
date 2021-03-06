package sync

import (
	"fmt"
	"math"
	"time"

	"github.com/ElrondNetwork/elrond-go/consensus"
	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-go/data"
	"github.com/ElrondNetwork/elrond-go/data/block"
	"github.com/ElrondNetwork/elrond-go/data/state"
	"github.com/ElrondNetwork/elrond-go/data/typeConverters/uint64ByteSlice"
	"github.com/ElrondNetwork/elrond-go/dataRetriever"
	"github.com/ElrondNetwork/elrond-go/hashing"
	"github.com/ElrondNetwork/elrond-go/marshal"
	"github.com/ElrondNetwork/elrond-go/process"
	"github.com/ElrondNetwork/elrond-go/process/factory"
	"github.com/ElrondNetwork/elrond-go/sharding"
	"github.com/ElrondNetwork/elrond-go/statusHandler"
	"github.com/ElrondNetwork/elrond-go/storage"
)

// MetaBootstrap implements the bootstrap mechanism
type MetaBootstrap struct {
	*baseBootstrap

	resolversFinder dataRetriever.ResolversFinder
	hdrRes          dataRetriever.HeaderResolver
}

// NewMetaBootstrap creates a new Bootstrap object
func NewMetaBootstrap(
	poolsHolder dataRetriever.MetaPoolsHolder,
	store dataRetriever.StorageService,
	blkc data.ChainHandler,
	rounder consensus.Rounder,
	blkExecutor process.BlockProcessor,
	waitTime time.Duration,
	hasher hashing.Hasher,
	marshalizer marshal.Marshalizer,
	forkDetector process.ForkDetector,
	resolversFinder dataRetriever.ResolversFinder,
	shardCoordinator sharding.Coordinator,
	accounts state.AccountsAdapter,
	bootstrapRoundIndex uint64,
) (*MetaBootstrap, error) {

	if poolsHolder == nil || poolsHolder.IsInterfaceNil() {
		return nil, process.ErrNilPoolsHolder
	}
	if poolsHolder.HeadersNonces() == nil || poolsHolder.HeadersNonces().IsInterfaceNil() {
		return nil, process.ErrNilHeadersNoncesDataPool
	}
	if poolsHolder.MetaBlocks() == nil || poolsHolder.MetaBlocks().IsInterfaceNil() {
		return nil, process.ErrNilMetaBlockPool
	}

	err := checkBootstrapNilParameters(
		blkc,
		rounder,
		blkExecutor,
		hasher,
		marshalizer,
		forkDetector,
		resolversFinder,
		shardCoordinator,
		accounts,
		store,
	)
	if err != nil {
		return nil, err
	}

	base := &baseBootstrap{
		blkc:                blkc,
		blkExecutor:         blkExecutor,
		store:               store,
		headers:             poolsHolder.MetaBlocks(),
		headersNonces:       poolsHolder.HeadersNonces(),
		rounder:             rounder,
		waitTime:            waitTime,
		hasher:              hasher,
		marshalizer:         marshalizer,
		forkDetector:        forkDetector,
		shardCoordinator:    shardCoordinator,
		accounts:            accounts,
		bootstrapRoundIndex: bootstrapRoundIndex,
	}

	boot := MetaBootstrap{
		baseBootstrap: base,
	}

	base.storageBootstrapper = &boot
	base.getHeaderFromPool = boot.getMetaHeaderFromPool

	//there is one header topic so it is ok to save it
	hdrResolver, err := resolversFinder.MetaChainResolver(factory.MetachainBlocksTopic)
	if err != nil {
		return nil, err
	}

	//placed in struct fields for performance reasons
	hdrRes, ok := hdrResolver.(dataRetriever.HeaderResolver)
	if !ok {
		return nil, process.ErrWrongTypeAssertion
	}
	boot.hdrRes = hdrRes

	boot.chRcvHdrNonce = make(chan bool)
	boot.chRcvHdrHash = make(chan bool)

	boot.setRequestedHeaderNonce(nil)
	boot.setRequestedHeaderHash(nil)
	boot.headersNonces.RegisterHandler(boot.receivedHeaderNonce)
	boot.headers.RegisterHandler(boot.receivedHeader)

	boot.chStopSync = make(chan bool)

	boot.statusHandler = statusHandler.NewNilStatusHandler()

	boot.syncStateListeners = make([]func(bool), 0)
	boot.requestedHashes = process.RequiredDataPool{}

	//TODO: This should be injected when BlockProcessor will be refactored
	boot.uint64Converter = uint64ByteSlice.NewBigEndianConverter()

	return &boot, nil
}

func (boot *MetaBootstrap) syncFromStorer(
	blockFinality uint64,
	blockUnit dataRetriever.UnitType,
	hdrNonceHashDataUnit dataRetriever.UnitType,
	notarizedBlockFinality uint64,
) error {
	err := boot.loadBlocks(
		blockFinality,
		blockUnit,
		hdrNonceHashDataUnit)
	if err != nil {
		return err
	}

	return nil
}

func (boot *MetaBootstrap) addHeaderToForkDetector(shardId uint32, nonce uint64, lastNotarizedMeta uint64) {
	header, headerHash, errNotCritical := boot.storageBootstrapper.getHeader(shardId, nonce)
	if errNotCritical != nil {
		log.Info(errNotCritical.Error())
		return
	}

	if shardId == sharding.MetachainShardId {
		errNotCritical = boot.forkDetector.AddHeader(header, headerHash, process.BHProcessed, nil, nil)
		if errNotCritical != nil {
			log.Debug(errNotCritical.Error())
		}

		return
	}
}

func (boot *MetaBootstrap) getHeader(shardId uint32, nonce uint64) (data.HeaderHandler, []byte, error) {
	return boot.getMetaHeaderFromStorage(shardId, nonce)
}

func (boot *MetaBootstrap) getBlockBody(headerHandler data.HeaderHandler) (data.BodyHandler, error) {
	return &block.MetaBlockBody{}, nil
}

func (boot *MetaBootstrap) removeBlockBody(
	nonce uint64,
	blockUnit dataRetriever.UnitType,
	hdrNonceHashDataUnit dataRetriever.UnitType,
) error {
	return nil
}

func (boot *MetaBootstrap) getNonceWithLastNotarized(nonce uint64) (uint64, map[uint32]uint64, map[uint32]uint64) {
	ni := notarizedInfo{}
	ni.reset()
	for currentNonce := nonce; currentNonce > 0; currentNonce-- {
		metaBlock, ok := boot.isMetaBlockValid(currentNonce)
		if !ok {
			ni.reset()
			continue
		}

		if ni.startNonce == 0 {
			ni.startNonce = currentNonce
		}

		if len(metaBlock.ShardInfo) == 0 {
			continue
		}

		maxNonce, err := boot.getMaxNotarizedHeadersNoncesInMetaBlock(metaBlock, &ni)
		if err != nil {
			log.Info(err.Error())
			ni.reset()
			continue
		}

		if boot.areNotarizedShardHeadersFound(&ni, maxNonce, currentNonce) {
			break
		}
	}

	log.Info(fmt.Sprintf("bootstrap from meta block with nonce %d\n", ni.startNonce))

	for i := uint32(0); i < boot.shardCoordinator.NumberOfShards(); i++ {
		if nonce > ni.blockWithLastNotarized[i] {
			ni.finalNotarized[i] = ni.lastNotarized[i]
		}

		log.Info(fmt.Sprintf("last notarized block from shard %d is %d and final notarized block is %d\n",
			i, ni.lastNotarized[i], ni.finalNotarized[i]))
	}

	return ni.startNonce, ni.finalNotarized, ni.lastNotarized
}

func (boot *MetaBootstrap) isMetaBlockValid(nonce uint64) (*block.MetaBlock, bool) {
	headerHandler, _, err := boot.getHeader(sharding.MetachainShardId, nonce)
	if err != nil {
		log.Info(err.Error())
		return nil, false
	}

	metaBlock, ok := headerHandler.(*block.MetaBlock)
	if !ok {
		log.Info(process.ErrWrongTypeAssertion.Error())
		return nil, false
	}

	if metaBlock.Round > boot.bootstrapRoundIndex {
		log.Info(ErrHigherRoundInBlock.Error())
		return nil, false
	}

	return metaBlock, true
}

func (boot *MetaBootstrap) getMaxNotarizedHeadersNoncesInMetaBlock(
	metaBlock *block.MetaBlock,
	ni *notarizedInfo,
) (map[uint32]uint64, error) {

	maxNonce := make(map[uint32]uint64, 0)
	for _, shardData := range metaBlock.ShardInfo {
		header, err := process.GetShardHeaderFromStorage(shardData.HeaderHash, boot.marshalizer, boot.store)
		if err != nil {
			return maxNonce, err
		}

		if header.Nonce > maxNonce[shardData.ShardId] {
			maxNonce[shardData.ShardId] = header.Nonce
		}
	}

	return maxNonce, nil
}

func (boot *MetaBootstrap) areNotarizedShardHeadersFound(
	ni *notarizedInfo,
	notarizedNonce map[uint32]uint64,
	nonce uint64,
) bool {

	for i := uint32(0); i < boot.shardCoordinator.NumberOfShards(); i++ {
		if ni.lastNotarized[i] == 0 {
			ni.lastNotarized[i] = notarizedNonce[i]
			ni.blockWithLastNotarized[i] = nonce
			continue
		}

		if ni.finalNotarized[i] == 0 {
			ni.finalNotarized[i] = notarizedNonce[i]
			ni.blockWithFinalNotarized[i] = nonce
			continue
		}
	}

	foundAllNotarizedShardHeaders := true
	for i := uint32(0); i < boot.shardCoordinator.NumberOfShards(); i++ {
		if ni.lastNotarized[i] == 0 || ni.finalNotarized[i] == 0 {
			foundAllNotarizedShardHeaders = false
			break
		}
	}

	return foundAllNotarizedShardHeaders
}

func (boot *MetaBootstrap) applyNotarizedBlocks(
	finalNotarized map[uint32]uint64,
	lastNotarized map[uint32]uint64,
) error {
	for i := uint32(0); i < boot.shardCoordinator.NumberOfShards(); i++ {
		nonce := finalNotarized[i]
		if nonce > 0 {
			headerHandler, _, err := boot.getShardHeaderFromStorage(i, nonce)
			if err != nil {
				return err
			}

			boot.blkExecutor.AddLastNotarizedHdr(i, headerHandler)
		}

		nonce = lastNotarized[i]
		if nonce > 0 {
			headerHandler, _, err := boot.getShardHeaderFromStorage(i, nonce)
			if err != nil {
				return err
			}

			boot.blkExecutor.AddLastNotarizedHdr(i, headerHandler)
		}
	}

	return nil
}

func (boot *MetaBootstrap) cleanupNotarizedStorage(lastNotarized map[uint32]uint64) {
	for i := uint32(0); i < boot.shardCoordinator.NumberOfShards(); i++ {
		hdrNonceHashDataUnit := dataRetriever.ShardHdrNonceHashDataUnit + dataRetriever.UnitType(i)
		highestNonceInStorer := boot.computeHighestNonce(hdrNonceHashDataUnit)

		for i := lastNotarized[i] + 1; i <= highestNonceInStorer; i++ {
			errNotCritical := boot.removeBlockHeader(i, dataRetriever.BlockHeaderUnit, hdrNonceHashDataUnit)
			if errNotCritical != nil {
				log.Info(fmt.Sprintf("remove notarized block header with nonce %d: %s\n", i, errNotCritical.Error()))
			}
		}
	}
}

func (boot *MetaBootstrap) receivedHeader(headerHash []byte) {
	header, err := process.GetMetaHeaderFromPool(headerHash, boot.headers)
	if err != nil {
		log.Debug(err.Error())
		return
	}

	boot.processReceivedHeader(header, headerHash)
}

// StartSync method will start SyncBlocks as a go routine
func (boot *MetaBootstrap) StartSync() {
	// when a node starts it first tries to bootstrap from storage, if there already exist a database saved
	errNotCritical := boot.syncFromStorer(process.MetaBlockFinality,
		dataRetriever.MetaBlockUnit,
		dataRetriever.MetaHdrNonceHashDataUnit,
		process.ShardBlockFinality)
	if errNotCritical != nil {
		log.Info(errNotCritical.Error())
	}

	go boot.syncBlocks()
}

// StopSync method will stop SyncBlocks
func (boot *MetaBootstrap) StopSync() {
	boot.chStopSync <- true
}

// syncBlocks method calls repeatedly synchronization method SyncBlock
func (boot *MetaBootstrap) syncBlocks() {
	for {
		time.Sleep(sleepTime)
		select {
		case <-boot.chStopSync:
			return
		default:
			err := boot.SyncBlock()
			if err != nil {
				log.Info(err.Error())
			}
		}
	}
}

func (boot *MetaBootstrap) doJobOnSyncBlockFail(hdr *block.MetaBlock, err error) {
	if err == process.ErrTimeIsOut {
		boot.requestsWithTimeout++
	}

	allowedRequestsWithTimeOutHaveReached := boot.requestsWithTimeout >= process.MaxRequestsWithTimeoutAllowed
	isInProperRound := process.IsInProperRound(boot.rounder.Index())

	shouldRollBack := err != process.ErrTimeIsOut || (allowedRequestsWithTimeOutHaveReached && isInProperRound)
	if shouldRollBack {
		boot.requestsWithTimeout = 0

		if hdr != nil {
			hash := boot.removeHeaderFromPools(hdr)
			boot.forkDetector.RemoveHeaders(hdr.Nonce, hash)
		}

		if allowedRequestsWithTimeOutHaveReached && isInProperRound {
			boot.forkDetector.ResetProbableHighestNonce()
		}

		errNotCritical := boot.forkChoice(false)
		if errNotCritical != nil {
			log.Info(errNotCritical.Error())
		}
	}
}

// SyncBlock method actually does the synchronization. It requests the next block header from the pool
// and if it is not found there it will be requested from the network. After the header is received,
// it requests the block body in the same way(pool and than, if it is not found in the pool, from network).
// If either header and body are received the ProcessAndCommit method will be called. This method will execute
// the block and its transactions. Finally if everything works, the block will be committed in the blockchain,
// and all this mechanism will be reiterated for the next block.
func (boot *MetaBootstrap) SyncBlock() error {
	if !boot.ShouldSync() {
		return nil
	}

	if boot.isForkDetected {
		isForcedFork := boot.forkNonce == math.MaxUint64 && boot.forkHash == nil

		if isForcedFork {
			log.Info(fmt.Sprintf("fork has been forced\n"))
		} else {
			log.Info(fmt.Sprintf("fork detected at nonce %d with hash %s\n",
				boot.forkNonce,
				core.ToB64(boot.forkHash)))
		}

		boot.statusHandler.Increment(core.MetricNumTimesInForkChoice)

		err := boot.forkChoice(!isForcedFork)
		if err != nil {
			log.Info(err.Error())
		}

		if isForcedFork {
			boot.forkDetector.ResetProbableHighestNonce()
			boot.forkDetector.ResetForcedFork()
			return nil
		}
	}

	boot.setRequestedHeaderNonce(nil)
	boot.setRequestedHeaderHash(nil)

	nonce := boot.getNonceForNextBlock()

	var hdr *block.MetaBlock
	var err error

	defer func() {
		if err != nil {
			boot.doJobOnSyncBlockFail(hdr, err)
		}
	}()

	if boot.isForkDetected {
		hdr, err = boot.getHeaderWithHashRequestingIfMissing(boot.forkHash)
	} else {
		hdr, err = boot.getHeaderWithNonceRequestingIfMissing(nonce)
	}

	if err != nil {
		boot.forkDetector.ResetProbableHighestNonceIfNeeded()
		return err
	}

	go boot.requestHeadersFromNonceIfMissing(hdr.GetNonce()+1, boot.haveMetaHeaderInPoolWithNonce, boot.hdrRes)

	haveTime := func() time.Duration {
		return boot.rounder.TimeDuration()
	}

	blockBody := &block.MetaBlockBody{}
	timeBefore := time.Now()
	err = boot.blkExecutor.ProcessBlock(boot.blkc, hdr, blockBody, haveTime)
	if err != nil {
		return err
	}
	timeAfter := time.Now()
	log.Info(fmt.Sprintf("time elapsed to process block: %v sec\n", timeAfter.Sub(timeBefore).Seconds()))

	timeBefore = time.Now()
	err = boot.blkExecutor.CommitBlock(boot.blkc, hdr, blockBody)
	if err != nil {
		return err
	}
	timeAfter = time.Now()
	log.Info(fmt.Sprintf("time elapsed to commit block: %v sec\n", timeAfter.Sub(timeBefore).Seconds()))

	log.Info(fmt.Sprintf("block with nonce %d has been synced successfully\n", hdr.Nonce))
	boot.requestsWithTimeout = 0

	return nil
}

// requestHeaderWithNonce method requests a block header from network when it is not found in the pool
func (boot *MetaBootstrap) requestHeaderWithNonce(nonce uint64) {
	boot.setRequestedHeaderNonce(&nonce)
	err := boot.hdrRes.RequestDataFromNonce(nonce)

	log.Info(fmt.Sprintf("requested header with nonce %d from network and probable highest nonce is %d\n",
		nonce,
		boot.forkDetector.ProbableHighestNonce()))

	if err != nil {
		log.Error(err.Error())
	}
}

// requestHeaderWithHash method requests a block header from network when it is not found in the pool
func (boot *MetaBootstrap) requestHeaderWithHash(hash []byte) {
	boot.setRequestedHeaderHash(hash)
	err := boot.hdrRes.RequestDataFromHash(hash)

	log.Info(fmt.Sprintf("requested header with hash %s from network\n", core.ToB64(hash)))

	if err != nil {
		log.Error(err.Error())
	}
}

// getHeaderWithNonceRequestingIfMissing method gets the header with a given nonce from pool. If it is not found there, it will
// be requested from network
func (boot *MetaBootstrap) getHeaderWithNonceRequestingIfMissing(nonce uint64) (*block.MetaBlock, error) {
	hdr, _, err := process.GetMetaHeaderFromPoolWithNonce(
		nonce,
		boot.headers,
		boot.headersNonces)
	if err != nil {
		_ = process.EmptyChannel(boot.chRcvHdrNonce)
		boot.requestHeaderWithNonce(nonce)
		err := boot.waitForHeaderNonce()
		if err != nil {
			return nil, err
		}

		hdr, _, err = process.GetMetaHeaderFromPoolWithNonce(
			nonce,
			boot.headers,
			boot.headersNonces)
		if err != nil {
			return nil, err
		}
	}

	return hdr, nil
}

// getHeaderWithHashRequestingIfMissing method gets the header with a given hash from pool. If it is not found there,
// it will be requested from network
func (boot *MetaBootstrap) getHeaderWithHashRequestingIfMissing(hash []byte) (*block.MetaBlock, error) {
	hdr, err := process.GetMetaHeader(hash, boot.headers, boot.marshalizer, boot.store)
	if err != nil {
		_ = process.EmptyChannel(boot.chRcvHdrHash)
		boot.requestHeaderWithHash(hash)
		err := boot.waitForHeaderHash()
		if err != nil {
			return nil, err
		}

		hdr, err = process.GetMetaHeaderFromPool(hash, boot.headers)
		if err != nil {
			return nil, err
		}
	}

	return hdr, nil
}

// forkChoice decides if rollback must be called
func (boot *MetaBootstrap) forkChoice(revertUsingForkNonce bool) error {
	log.Info("starting fork choice\n")
	for {
		header, err := boot.getCurrentHeader()
		if err != nil {
			return err
		}

		if !revertUsingForkNonce && header.Nonce <= boot.forkDetector.GetHighestFinalBlockNonce() {
			return ErrRollBackBehindFinalHeader
		}

		log.Info(fmt.Sprintf("roll back to header with nonce %d and hash %s as the highest final block nonce is %d\n",
			header.Nonce-1,
			core.ToB64(header.GetPrevHash()),
			boot.forkDetector.GetHighestFinalBlockNonce()))

		err = boot.rollback(header)
		if err != nil {
			return err
		}

		if revertUsingForkNonce && header.Nonce > boot.forkNonce {
			continue
		}

		break
	}

	log.Info("ending fork choice\n")
	return nil
}

func (boot *MetaBootstrap) rollback(header *block.MetaBlock) error {
	if header.GetNonce() == 0 {
		return process.ErrRollbackFromGenesis
	}

	headerStore := boot.store.GetStorer(dataRetriever.MetaBlockUnit)
	if headerStore == nil {
		return process.ErrNilHeadersStorage
	}

	headerNonceHashStore := boot.store.GetStorer(dataRetriever.MetaHdrNonceHashDataUnit)
	if headerNonceHashStore == nil {
		return process.ErrNilHeadersNonceHashStorage
	}

	var err error
	var newHeader *block.MetaBlock
	var newHeaderHash []byte
	var newRootHash []byte

	if header.GetNonce() > 1 {
		newHeader, err = boot.getPrevHeader(headerStore, header)
		if err != nil {
			return err
		}

		newHeaderHash = header.GetPrevHash()
		newRootHash = newHeader.GetRootHash()
	} else { // rollback to genesis block
		newRootHash = boot.blkc.GetGenesisHeader().GetRootHash()
	}

	err = boot.blkc.SetCurrentBlockHeader(newHeader)
	if err != nil {
		return err
	}

	boot.blkc.SetCurrentBlockHeaderHash(newHeaderHash)

	err = boot.accounts.RecreateTrie(newRootHash)
	if err != nil {
		return err
	}

	boot.cleanCachesAndStorageOnRollback(header, headerStore, headerNonceHashStore)
	errNotCritical := boot.blkExecutor.RestoreBlockIntoPools(header, nil)
	if errNotCritical != nil {
		log.Info(errNotCritical.Error())
	}

	return nil
}

func (boot *MetaBootstrap) getPrevHeader(headerStore storage.Storer, header *block.MetaBlock) (*block.MetaBlock, error) {
	prevHash := header.GetPrevHash()
	buffHeader, err := headerStore.Get(prevHash)
	if err != nil {
		return nil, err
	}

	newHeader := &block.MetaBlock{}
	err = boot.marshalizer.Unmarshal(newHeader, buffHeader)
	if err != nil {
		return nil, err
	}

	return newHeader, nil
}

func (boot *MetaBootstrap) getCurrentHeader() (*block.MetaBlock, error) {
	blockHeader := boot.blkc.GetCurrentBlockHeader()
	if blockHeader == nil {
		return nil, process.ErrNilBlockHeader
	}

	header, ok := blockHeader.(*block.MetaBlock)
	if !ok {
		return nil, process.ErrWrongTypeAssertion
	}

	return header, nil
}

// IsInterfaceNil returns true if there is no value under the interface
func (boot *MetaBootstrap) IsInterfaceNil() bool {
	if boot == nil {
		return true
	}
	return false
}

func (boot *MetaBootstrap) haveMetaHeaderInPoolWithNonce(nonce uint64) bool {
	_, _, err := process.GetMetaHeaderFromPoolWithNonce(
		nonce,
		boot.headers,
		boot.headersNonces)

	return err == nil
}

func (boot *MetaBootstrap) getMetaHeaderFromPool(headerHash []byte) (data.HeaderHandler, error) {
	header, err := process.GetMetaHeaderFromPool(
		headerHash,
		boot.headers)

	return header, err
}
