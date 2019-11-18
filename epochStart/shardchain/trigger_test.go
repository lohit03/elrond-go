package shardchain

import (
	"testing"

	"github.com/ElrondNetwork/elrond-go/data"
	"github.com/ElrondNetwork/elrond-go/data/block"
	"github.com/ElrondNetwork/elrond-go/dataRetriever"
	"github.com/ElrondNetwork/elrond-go/epochStart"
	"github.com/ElrondNetwork/elrond-go/epochStart/mock"
	"github.com/ElrondNetwork/elrond-go/storage"
	"github.com/stretchr/testify/assert"
)

func createMockShardEpochStartTriggerArguments() *ArgsShardEpochStartTrigger {
	return &ArgsShardEpochStartTrigger{
		Marshalizer: &mock.MarshalizerMock{},
		Hasher:      &mock.HasherMock{},
		HeaderValidator: &mock.HeaderValidatorStub{
			IsHeaderConstructionValidCalled: func(currHdr, prevHdr data.HeaderHandler) error {
				return nil
			},
		},
		Uint64Converter: &mock.Uint64ByteSliceConverterMock{},
		DataPool: &mock.PoolsHolderStub{
			MetaBlocksCalled: func() storage.Cacher {
				return &mock.CacherStub{
					PeekCalled: func(key []byte) (value interface{}, ok bool) {
						return nil, true
					},
				}
			},
			HeadersNoncesCalled: func() dataRetriever.Uint64SyncMapCacher {
				return &mock.Uint64SyncMapCacherStub{
					GetCalled: func(nonce uint64) (hashMap dataRetriever.ShardIdHashMap, b bool) {
						return &mock.ShardIdHasMapStub{LoadCalled: func(shardId uint32) (bytes []byte, b bool) {
							return []byte("hash"), true
						}}, true
					},
				}
			},
		},
		Storage: &mock.ChainStorerStub{
			GetStorerCalled: func(unitType dataRetriever.UnitType) storage.Storer {
				return &mock.StorerStub{
					GetCalled: func(key []byte) (bytes []byte, err error) {
						return []byte("hash"), nil
					},
				}
			},
		},
		RequestHandler: &mock.RequestHandlerStub{},
	}
}

func TestNewEpochStartTrigger_NilArgumentsShouldErr(t *testing.T) {
	t.Parallel()

	eoet, err := NewEpochStartTrigger(nil)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilArgsNewShardEpochStartTrigger, err)
}

func TestNewEpochStartTrigger_NilHasherShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Hasher = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilHasher, err)
}

func TestNewEpochStartTrigger_NilMarshalizerShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Marshalizer = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilMarshalizer, err)
}

func TestNewEpochStartTrigger_NilHeaderShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.HeaderValidator = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilHeaderValidator, err)
}

func TestNewEpochStartTrigger_NilDataPoolShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.DataPool = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilDataPoolsHolder, err)
}

func TestNewEpochStartTrigger_NilStorageShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Storage = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilStorageService, err)
}

func TestNewEpochStartTrigger_NilRequestHandlerShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.RequestHandler = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilRequestHandler, err)
}

func TestNewEpochStartTrigger_NilMetaBlockPoolShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.DataPool = &mock.PoolsHolderStub{
		MetaBlocksCalled: func() storage.Cacher {
			return nil
		},
	}
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilMetaBlocksPool, err)
}

func TestNewEpochStartTrigger_NilHeadersNonceShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.DataPool = &mock.PoolsHolderStub{
		MetaBlocksCalled: func() storage.Cacher {
			return &mock.CacherStub{}
		},
		HeadersNoncesCalled: func() dataRetriever.Uint64SyncMapCacher {
			return nil
		},
	}
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilHeaderNoncesPool, err)
}

func TestNewEpochStartTrigger_NilUint64ConverterShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Uint64Converter = nil
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilUint64Converter, err)
}

func TestNewEpochStartTrigger_NilMetaBlockUnitShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Storage = &mock.ChainStorerStub{
		GetStorerCalled: func(unitType dataRetriever.UnitType) storage.Storer {
			return nil
		},
	}
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilMetaHdrStorage, err)
}

func TestNewEpochStartTrigger_NilMetaNonceHashStorageShouldErr(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Storage = &mock.ChainStorerStub{
		GetStorerCalled: func(unitType dataRetriever.UnitType) storage.Storer {
			switch unitType {
			case dataRetriever.MetaHdrNonceHashDataUnit:
				return nil
			default:
				return &mock.StorerStub{}
			}
		},
	}
	eoet, err := NewEpochStartTrigger(args)

	assert.Nil(t, eoet)
	assert.Equal(t, epochStart.ErrNilMetaNonceHashStorage, err)
}

func TestNewEpochStartTrigger_ShouldOk(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	eoet, err := NewEpochStartTrigger(args)

	assert.NotNil(t, eoet)
	assert.Nil(t, err)
}

func TestTrigger_ReceivedHeaderNotEpochStart(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Validity = 2
	args.Finality = 2
	eoet, _ := NewEpochStartTrigger(args)

	hash := []byte("hash")
	header := &block.MetaBlock{Nonce: 100}
	header.EpochStart.LastFinalizedHeaders = []block.EpochStartShardData{{ShardId: 0, RootHash: hash, HeaderHash: hash}}
	eoet.ReceivedHeader(header)

	assert.False(t, eoet.IsEpochStart())
}

func TestTrigger_ReceivedHeaderIsEpochStartTrue(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Validity = 0
	args.Finality = 2
	eoet, _ := NewEpochStartTrigger(args)

	hash := []byte("hash")
	header := &block.MetaBlock{Nonce: 100, Epoch: 1}
	header.EpochStart.LastFinalizedHeaders = []block.EpochStartShardData{{ShardId: 0, RootHash: hash, HeaderHash: hash}}
	eoet.ReceivedHeader(header)

	header = &block.MetaBlock{Nonce: 101, Epoch: 1}
	eoet.ReceivedHeader(header)

	assert.True(t, eoet.IsEpochStart())
}

func TestTrigger_Epoch(t *testing.T) {
	t.Parallel()

	epoch := uint32(1)
	args := createMockShardEpochStartTriggerArguments()
	args.Epoch = epoch
	eoet, _ := NewEpochStartTrigger(args)

	currentEpoch := eoet.Epoch()
	assert.Equal(t, epoch, currentEpoch)
}

func TestTrigger_ProcessedAndRevert(t *testing.T) {
	t.Parallel()

	args := createMockShardEpochStartTriggerArguments()
	args.Validity = 0
	args.Finality = 0
	et, _ := NewEpochStartTrigger(args)

	hash := []byte("hash")
	epochStartRound := uint64(100)
	header := &block.MetaBlock{Nonce: 100, Round: epochStartRound, Epoch: 1}
	header.EpochStart.LastFinalizedHeaders = []block.EpochStartShardData{{ShardId: 0, RootHash: hash, HeaderHash: hash}}
	et.ReceivedHeader(header)
	header = &block.MetaBlock{Nonce: 101, Round: epochStartRound + 1, Epoch: 1}
	et.ReceivedHeader(header)

	assert.True(t, et.IsEpochStart())
	assert.Equal(t, epochStartRound, et.EpochStartRound())

	et.Processed()
	assert.False(t, et.isEpochStart)
	assert.False(t, et.newEpochHdrReceived)

	et.Revert()
	assert.True(t, et.isEpochStart)
	assert.True(t, et.newEpochHdrReceived)
}