package replica

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/constants"
	"github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/filestore"
	"github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/keys"
	"github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/rbuilder"
	"github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/sender"
	"github.com/buildbuddy-io/buildbuddy/server/util/log"
	"github.com/buildbuddy-io/buildbuddy/server/util/rangemap"
	"github.com/buildbuddy-io/buildbuddy/server/util/status"
	"github.com/cockroachdb/pebble"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	rfpb "github.com/buildbuddy-io/buildbuddy/proto/raft"
	dbsm "github.com/lni/dragonboat/v3/statemachine"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	gstatus "google.golang.org/grpc/status"
)

const (
	peerReadTimeout = 60 * time.Second
)

type IStore interface {
	AddRange(rd *rfpb.RangeDescriptor, r *Replica)
	RemoveRange(rd *rfpb.RangeDescriptor, r *Replica)
	Sender() *sender.Sender
	ReadFileFromPeer(ctx context.Context, except *rfpb.ReplicaDescriptor, fileRecord *rfpb.FileRecord) (io.ReadCloser, error)
	GetReplica(rangeID uint64) (*Replica, error)
}

// IOnDiskStateMachine is the interface to be implemented by application's
// state machine when the state machine state is always persisted on disks.
// IOnDiskStateMachine basically matches the state machine type described
// in the section 5.2 of the Raft thesis.
//
// For IOnDiskStateMachine types, concurrent access to the state machine is
// supported. An IOnDiskStateMachine type allows its Update method to be
// concurrently invoked when there are ongoing calls to the Lookup or the
// SaveSnapshot method. Lookup is also allowed when the RecoverFromSnapshot or
// the Close methods are being invoked. Invocations to the Update, Sync,
// PrepareSnapshot, RecoverFromSnapshot and Close methods are guarded by the
// system to ensure mutual exclusion.
//
// Once created, the Open method is immediately invoked to open and use the
// persisted state on disk. This makes IOnDiskStateMachine different from
// IStateMachine types which require the state machine state to be fully
// reconstructed from saved snapshots and Raft logs.
//
// Applications that implement IOnDiskStateMachine are recommended to setup
// periodic snapshotting with relatively short time intervals, that triggers
// the state machine's metadata, usually only a few KBytes each, to be
// periodically snapshotted and thus causes negligible overheads for the system.
// It also provides opportunities for the system to signal Raft Log compactions
// to free up disk spaces.
type Replica struct {
	db       *pebble.DB
	closedMu *sync.RWMutex // PROTECTS(closed)
	closed   bool
	wg       sync.WaitGroup

	rootDir   string
	fileDir   string
	clusterID uint64
	nodeID    uint64

	store            IStore
	lastAppliedIndex uint64
	splitMu          sync.RWMutex

	log             log.Logger
	rangeMu         sync.RWMutex
	rangeDescriptor *rfpb.RangeDescriptor
	mappedRange     *rangemap.Range
}

func uint64ToBytes(i uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, i)
	return buf
}

func bytesToUint64(buf []byte) uint64 {
	return binary.LittleEndian.Uint64(buf)
}

func batchLookup(wb *pebble.Batch, query []byte) ([]byte, error) {
	buf, closer, err := wb.Get(query)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, status.NotFoundErrorf("Key not found: %s", err)
		}
		return nil, err
	}
	defer closer.Close()
	if len(buf) == 0 {
		return nil, status.NotFoundError("Key not found (empty)")
	}

	// We need to copy the value from pebble before
	// closer is closed.
	val := make([]byte, len(buf))
	copy(val, buf)
	return val, nil
}

func (sm *Replica) Usage() (*rfpb.ReplicaUsage, error) {
	ru := &rfpb.ReplicaUsage{
		Replica: &rfpb.ReplicaDescriptor{
			ClusterId: sm.clusterID,
			NodeId:    sm.nodeID,
		},
	}
	if sm.db != nil {
		du, err := sm.db.EstimateDiskUsage(keys.Key{constants.MinByte}, keys.Key{constants.MaxByte})
		if err != nil {
			log.Errorf("Error estimating disk usage: %s", err)
			return nil, err
		}
		ru.EstimatedDiskBytesUsed = int64(du)
	}
	return ru, nil
}

func (sm *Replica) setRange(key, val []byte) error {
	if bytes.Compare(key, constants.LocalRangeKey) != 0 {
		return status.FailedPreconditionErrorf("setRange called with non-range key: %s", key)
	}

	rangeDescriptor := &rfpb.RangeDescriptor{}
	if err := proto.Unmarshal(val, rangeDescriptor); err != nil {
		return err
	}

	sm.rangeMu.Lock()
	defer sm.rangeMu.Unlock()

	if sm.rangeDescriptor != nil {
		sm.store.RemoveRange(sm.rangeDescriptor, sm)
	}

	sm.rangeDescriptor = rangeDescriptor
	sm.mappedRange = &rangemap.Range{
		Left:  rangeDescriptor.GetLeft(),
		Right: rangeDescriptor.GetRight(),
	}
	sm.store.AddRange(sm.rangeDescriptor, sm)
	return nil
}

func (sm *Replica) rangeCheckedSet(wb *pebble.Batch, key, val []byte) error {
	sm.rangeMu.RLock()

	if !keys.IsLocalKey(key) {
		if sm.mappedRange != nil && sm.mappedRange.Contains(key) {
			sm.rangeMu.RUnlock()
			return wb.Set(key, val, nil /*ignored write options*/)
		}
		sm.rangeMu.RUnlock()
		return status.OutOfRangeErrorf("range %s does not contain key %q", sm.mappedRange, string(key))
	}
	sm.rangeMu.RUnlock()

	if err := wb.Set(key, val, nil /*ignored write options*/); err != nil {
		return err
	}
	if bytes.Compare(key, constants.LocalRangeKey) == 0 {
		if err := sm.setRange(key, val); err != nil {
			log.Errorf("Error setting range: %s", err)
		}
	}
	return nil
}

func (sm *Replica) lookup(db ReplicaReader, query []byte) ([]byte, error) {
	buf, closer, err := db.Get(query)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, status.NotFoundErrorf("Key not found: %s", err)
		}
		return nil, err
	}
	defer closer.Close()
	if len(buf) == 0 {
		return nil, status.NotFoundError("Key not found (empty)")
	}

	// We need to copy the value from pebble before
	// closer is closed.
	val := make([]byte, len(buf))
	copy(val, buf)
	return val, nil
}

func (sm *Replica) getLastAppliedIndex(db ReplicaReader) (uint64, error) {
	val, err := sm.lookup(db, []byte(constants.LastAppliedIndexKey))
	if err != nil {
		if status.IsNotFoundError(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(val) == 0 {
		return 0, nil
	}
	i := bytesToUint64(val)
	return i, nil
}

func (sm *Replica) getDBDir() string {
	return filepath.Join(sm.rootDir,
		fmt.Sprintf("cluster-%d", sm.clusterID),
		fmt.Sprintf("node-%d", sm.nodeID))
}

type ReplicaReader interface {
	pebble.Reader
	io.Closer
}

type ReplicaWriter interface {
	pebble.Writer
	io.Closer

	// Would prefer to just use pebble.Writer here but the interface offers
	// no functionality for actually creating a new batch, so we amend it.
	NewBatch() *pebble.Batch
	NewIndexedBatch() *pebble.Batch
	NewSnapshot() *pebble.Snapshot
}

type refCounter struct {
	wg     *sync.WaitGroup
	closed bool
}

func newRefCounter(wg *sync.WaitGroup) *refCounter {
	wg.Add(1)
	return &refCounter{
		wg:     wg,
		closed: false,
	}
}
func (r *refCounter) Close() error {
	if !r.closed {
		r.closed = true
		r.wg.Add(-1)
	}
	return nil
}

type refCountedDB struct {
	*pebble.DB
	*refCounter
}

func (r *refCountedDB) Close() error {
	// Just close the refcounter, not the DB.
	return r.refCounter.Close()
}

func (sm *Replica) Reader() (ReplicaReader, error) {
	sm.closedMu.RLock()
	defer sm.closedMu.RUnlock()
	if sm.closed {
		return nil, status.FailedPreconditionError("db is closed.")
	}

	return &refCountedDB{
		sm.db,
		newRefCounter(&sm.wg),
	}, nil
}

func (sm *Replica) DB() (ReplicaWriter, error) {
	sm.closedMu.RLock()
	defer sm.closedMu.RUnlock()
	if sm.closed {
		return nil, status.FailedPreconditionError("db is closed.")
	}

	// RLock the split lock. This prevents DB writes during a split.
	sm.splitMu.RLock()
	defer sm.splitMu.RUnlock()

	return &refCountedDB{
		sm.db,
		newRefCounter(&sm.wg),
	}, nil
}

// Open opens the existing on disk state machine to be used or it creates a
// new state machine with empty state if it does not exist. Open returns the
// most recent index value of the Raft log that has been persisted, or it
// returns 0 when the state machine is a new one.
//
// The provided read only chan struct{} channel is used to notify the Open
// method that the node has been stopped and the Open method can choose to
// abort by returning an ErrOpenStopped error.
//
// Open is called shortly after the Raft node is started. The Update method
// and the Lookup method will not be called before the completion of the Open
// method.
func (sm *Replica) Open(stopc <-chan struct{}) (uint64, error) {
	db, err := pebble.Open(sm.getDBDir(), &pebble.Options{})
	if err != nil {
		return 0, err
	}
	sm.closedMu.Lock()
	sm.db = db
	sm.closed = false
	sm.closedMu.Unlock()

	sm.checkAndSetRangeDescriptor(db)
	return sm.getLastAppliedIndex(db)
}

func (sm *Replica) checkAndSetRangeDescriptor(db ReplicaReader) {
	buf, err := sm.lookup(db, constants.LocalRangeKey)
	if err != nil {
		sm.log.Debugf("LocalRangeKey not found on replica")
		return
	}
	sm.setRange(constants.LocalRangeKey, buf)
}

func (sm *Replica) readFileFromPeer(ctx context.Context, fileRecord *rfpb.FileRecord, wb *pebble.Batch) (*rfpb.FileMetadata, error) {
	thisReplica := &rfpb.ReplicaDescriptor{
		ClusterId: sm.clusterID,
		NodeId:    sm.nodeID,
	}
	rc, err := sm.store.ReadFileFromPeer(ctx, thisReplica, fileRecord)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	wc, err := filestore.NewWriter(ctx, sm.fileDir, wb, fileRecord)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(wc, rc); err != nil {
		return nil, err
	}
	if err := wc.Close(); err != nil {
		return nil, err
	}

	return &rfpb.FileMetadata{
		FileRecord:      fileRecord,
		StorageMetadata: wc.Metadata(),
	}, nil
}

func (sm *Replica) fileWrite(wb *pebble.Batch, req *rfpb.FileWriteRequest) (*rfpb.FileWriteResponse, error) {
	// In the common case, this doesn't actually write any data, because
	// it's already been written it in a separate Write RPC. It simply
	// validates that the Metadata key exists. If it does not, the data is
	// read from a peer.
	iter := wb.NewIter(nil /*default iter options*/)
	defer iter.Close()

	fileMetadataKey, err := constants.FileMetadataKey(req.GetFileRecord())
	if err != nil {
		return nil, err
	}

	found := iter.SeekGE(fileMetadataKey)
	if !found || bytes.Compare(fileMetadataKey, iter.Key()) != 0 {
		md, err := sm.readFileFromPeer(context.TODO(), req.GetFileRecord(), wb)
		if err != nil {
			log.Errorf("Error recovering file %v from peer: %s", req.GetFileRecord(), err)
			return nil, err
		}
		protoBytes, err := proto.Marshal(md)
		if err := sm.rangeCheckedSet(wb, fileMetadataKey, protoBytes); err != nil {
			return nil, err
		}
	}
	return &rfpb.FileWriteResponse{}, nil

}

func (sm *Replica) directWrite(wb *pebble.Batch, req *rfpb.DirectWriteRequest) (*rfpb.DirectWriteResponse, error) {
	kv := req.GetKv()
	return &rfpb.DirectWriteResponse{}, sm.rangeCheckedSet(wb, kv.Key, kv.Value)
}

func (sm *Replica) directRead(db ReplicaReader, req *rfpb.DirectReadRequest) (*rfpb.DirectReadResponse, error) {
	buf, err := sm.lookup(db, req.GetKey())
	if err != nil {
		return nil, err
	}
	rsp := &rfpb.DirectReadResponse{
		Kv: &rfpb.KV{
			Key:   req.GetKey(),
			Value: buf,
		},
	}
	return rsp, nil
}

func (sm *Replica) increment(wb *pebble.Batch, req *rfpb.IncrementRequest) (*rfpb.IncrementResponse, error) {
	if len(req.GetKey()) == 0 {
		return nil, status.InvalidArgumentError("Increment requires a valid key.")
	}
	buf, err := batchLookup(wb, req.GetKey())
	if err != nil {
		if !status.IsNotFoundError(err) {
			return nil, err
		}
	}
	var val uint64
	if status.IsNotFoundError(err) {
		val = 0
	} else {
		val = bytesToUint64(buf)
	}
	val += req.GetDelta()

	if err := wb.Set(req.GetKey(), uint64ToBytes(val), nil /*ignored write options*/); err != nil {
		return nil, err
	}
	return &rfpb.IncrementResponse{
		Key:   req.GetKey(),
		Value: val,
	}, nil
}

func (sm *Replica) cas(wb *pebble.Batch, req *rfpb.CASRequest) (*rfpb.CASResponse, error) {
	kv := req.GetKv()
	var buf []byte
	var err error
	buf, err = sm.lookup(wb, kv.GetKey())
	if err != nil && !status.IsNotFoundError(err) {
		return nil, err
	}

	// Match: set value and return new value + no error.
	if bytes.Compare(buf, req.GetExpectedValue()) == 0 {
		err := sm.rangeCheckedSet(wb, kv.Key, kv.Value)
		if err == nil {
			return &rfpb.CASResponse{Kv: kv}, nil
		}
		return nil, err
	}

	// No match: return old value and error.
	return &rfpb.CASResponse{
		Kv: &rfpb.KV{
			Key:   kv.GetKey(),
			Value: buf,
		},
	}, status.FailedPreconditionError(constants.CASErrorMessage)
}

type splitPoint struct {
	left      []byte
	right     []byte
	leftSize  int64
	rightSize int64
}

func (sm *Replica) findSplitPoint(wb *pebble.Batch, req *rfpb.FindSplitPointRequest) (*rfpb.FindSplitPointResponse, error) {
	iterOpts := &pebble.IterOptions{
		LowerBound: keys.Key([]byte{constants.MinByte}),
		UpperBound: keys.Key([]byte{constants.MaxByte}),
	}

	iter := wb.NewIter(iterOpts)
	defer iter.Close()
	var t bool = iter.First()

	totalSize := int64(0)
	for ; t; t = iter.Next() {
		if t {
			totalSize += int64(len(iter.Value()))
		}
	}

	leftSplitSize := int64(0)
	t = iter.First()
	var lastKey []byte
	for ; t; t = iter.Next() {
		if leftSplitSize >= totalSize/2 && canSplitKeys(lastKey, iter.Key()) {
			sp := &rfpb.FindSplitPointResponse{
				Left:           make([]byte, len(lastKey)),
				LeftSizeBytes:  leftSplitSize,
				Right:          make([]byte, len(iter.Key())),
				RightSizeBytes: totalSize - leftSplitSize,
			}
			copy(sp.Left, lastKey)
			copy(sp.Right, iter.Key())
			log.Debugf("Found split point: %+v", sp)
			return sp, nil
		}
		if t {
			leftSplitSize += int64(len(iter.Value()))
			if len(lastKey) != len(iter.Key()) {
				lastKey = make([]byte, len(iter.Key()))
			}
			copy(lastKey, iter.Key())
		}
	}
	return nil, status.NotFoundErrorf("Could not find split point. (Total size: %d, left split size: %d", totalSize, leftSplitSize)
}

func canSplitKeys(leftKey, rightKey []byte) bool {
	splitStart := []byte{constants.UnsplittableMaxByte}
	// Disallow splitting the metarange, or any range before '\x04'.
	if bytes.Compare(rightKey, splitStart) <= 0 {
		log.Debugf("can't split between %q and %q, end in metarange", leftKey, rightKey)
		return false
	}

	if bytes.Compare(leftKey, splitStart) <= 0 && bytes.Compare(rightKey, splitStart) > 0 {
		log.Debugf("can't split between %q and %q, overlaps metarange", leftKey, rightKey)
		return false
	}

	// Disallow splitting pebble file-metadata from stored-file-data.
	// File mdata will have a key like /foo/bar/baz
	// File data will have a key like /foo/bar/baz-{1..n}
	if bytes.HasPrefix(rightKey, leftKey) {
		log.Debugf("can't split between %q and %q, prefix match", leftKey, rightKey)
		return false
	}

	return true
}

func (sm *Replica) populateDBFromSnapshot(sourceDB, destDB ReplicaWriter) error {
	// Create a snapshot of the left range.
	f, err := os.CreateTemp(sm.fileDir, "replica-*.snap")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	snap := sourceDB.NewSnapshot()
	defer snap.Close()
	if err := SaveSnapshotToWriter(f, snap); err != nil {
		return err
	}

	// Load the snapshot into the new right range.
	f.Seek(0, 0)
	return ApplySnapshotFromReader(f, destDB)
}

func (sm *Replica) updateMetarange(oldLeft, left, right *rfpb.RangeDescriptor) error {
	leftBuf, err := proto.Marshal(left)
	if err != nil {
		return err
	}
	oldLeftBuf, err := proto.Marshal(oldLeft)
	if err != nil {
		return err
	}
	rightBuf, err := proto.Marshal(right)
	if err != nil {
		return err
	}

	// Send a single request that:
	//  - CAS sets the left value to leftRDBuf
	//  - inserts the new rightBuf
	//
	// if the CAS fails, check the existing value
	//  if it's generation is past ours, ignore the error, we're out of date
	//  if the existing value already matches what we were trying to set, we're done.
	//  else return an error
	batchProto, err := rbuilder.NewBatchBuilder().Add(&rfpb.CASRequest{
		Kv: &rfpb.KV{
			Key:   keys.RangeMetaKey(right.GetRight()),
			Value: rightBuf,
		},
		ExpectedValue: oldLeftBuf,
	}).Add(&rfpb.DirectWriteRequest{
		Kv: &rfpb.KV{
			Key:   keys.RangeMetaKey(left.GetRight()),
			Value: leftBuf,
		},
	}).ToProto()
	if err != nil {
		return err
	}
	rsp, err := sm.store.Sender().SyncPropose(context.TODO(), keys.RangeMetaKey(right.GetRight()), batchProto)
	if err != nil {
		return err
	}
	batchRsp := rbuilder.NewBatchResponseFromProto(rsp)
	casRsp, err := batchRsp.CASResponse(0)
	if err != nil {
		if casRsp == nil {
			return err // shouldn't happen.
		}
		if bytes.Compare(casRsp.GetKv().GetValue(), rightBuf) == 0 {
			// another replica already applied the change.
			return nil
		}
		existingRange := &rfpb.RangeDescriptor{}
		if err := proto.Unmarshal(casRsp.GetKv().GetValue(), existingRange); err != nil {
			return err
		}
		if existingRange.GetGeneration() > right.GetGeneration() {
			// this replica is behind; don't need to update mr.
			return nil
		}
		return err // shouldn't happen.
	}
	return nil
}

func printRange(wb *pebble.Batch, tag string) {
	iter := wb.NewIter(&pebble.IterOptions{})
	for iter.First(); iter.Valid(); iter.Next() {
		log.Printf("%q: key: %q", tag, iter.Key())
	}
	defer iter.Close()
}

func (sm *Replica) split(wb *pebble.Batch, req *rfpb.SplitRequest) (*rfpb.SplitResponse, error) {
	if req.GetLeft() == nil || req.GetProposedRight() == nil {
		return nil, status.FailedPreconditionError("left and right ranges must be provided")
	}
	if req.GetSplitPoint() == nil {
		return nil, status.FailedPreconditionError("unable to split range: couldn't find split point")
	}

	leftDB, err := sm.DB()
	if err != nil {
		return nil, err
	}
	defer leftDB.Close()

	// Lock the range to external writes.
	sm.splitMu.Lock()
	defer sm.splitMu.Unlock()

	sm.rangeMu.Lock()
	rd := sm.rangeDescriptor
	sm.rangeMu.Unlock()

	if !proto.Equal(rd, req.GetLeft()) {
		rdText, _ := (&prototext.MarshalOptions{Multiline: false}).Marshal(rd)
		leftText, _ := (&prototext.MarshalOptions{Multiline: false}).Marshal(req.GetLeft())
		return nil, status.OutOfRangeErrorf("split %q (current) != %q (req)", string(rdText), string(leftText))
	}

	rightSM, err := sm.store.GetReplica(req.GetProposedRight().GetRangeId())
	if err != nil {
		return nil, err
	}

	// Populate the new replica.
	rightDB, err := rightSM.DB()
	if err != nil {
		return nil, err
	}
	defer rightDB.Close()

	if err := sm.populateDBFromSnapshot(leftDB, rightDB); err != nil {
		return nil, err
	}
	rwb := rightDB.NewIndexedBatch()
	defer rwb.Close()

	// Delete the keys from each side that are now owned by the other side.
	// Right side delete should be a no-op if this is a freshly created replica.
	sp := req.GetSplitPoint()
	rightDeleteEnd := keys.Key(sp.GetLeft()).Next()
	if err := rwb.DeleteRange(keys.Key{constants.MinByte}, rightDeleteEnd, nil /*ignored write options*/); err != nil {
		return nil, err
	}
	if err := wb.DeleteRange(sp.GetRight(), keys.Key{constants.MaxByte}, nil /*ignored write options*/); err != nil {
		return nil, err
	}

	// Write the updated local range keys to both places
	// Update the metarange and return any errors.
	leftRD := proto.Clone(req.GetLeft()).(*rfpb.RangeDescriptor)
	rightRD := proto.Clone(req.GetProposedRight()).(*rfpb.RangeDescriptor)
	leftRD.Generation += 1                     // increment rd generation upon split
	rightRD.Generation = leftRD.Generation + 1 // increment rd generation upon split
	rightRD.Right = req.GetLeft().GetRight()   // new range's end is the prev range's end
	rightRD.Left = sp.GetRight()               // new range's beginning is split point right side
	leftRD.Right = sp.GetLeft()                // old range's end is now split point left side
	rightRDBuf, err := proto.Marshal(rightRD)
	if err != nil {
		return nil, err
	}
	leftRDBuf, err := proto.Marshal(leftRD)
	if err != nil {
		return nil, err
	}

	// update the left local range (when this batch is committed).
	if err := sm.rangeCheckedSet(wb, constants.LocalRangeKey, leftRDBuf); err != nil {
		return nil, err
	}

	// update the right local range: this will make it active, but no
	// traffic will be sent yet because the metarange has not been updated.
	if err := rightSM.rangeCheckedSet(rwb, constants.LocalRangeKey, rightRDBuf); err != nil {
		return nil, err
	}

	if err := rightDB.Apply(rwb, &pebble.WriteOptions{Sync: true}); err != nil {
		return nil, err
	}

	// if left limit is < constants.UnsplittableMaxByte, then we own the
	// metarange, so update it here.
	unsplittable := []byte{constants.UnsplittableMaxByte}
	if bytes.Compare(rd.GetLeft(), unsplittable) == -1 {
		// we own the metarange, so update it here.
		if err := sm.rangeCheckedSet(wb, keys.RangeMetaKey(rightRD.Right), rightRDBuf); err != nil {
			return nil, err
		}
		if err := sm.rangeCheckedSet(wb, keys.RangeMetaKey(leftRD.Right), leftRDBuf); err != nil {
			return nil, err
		}
	} else {
		// use sender to remotely update the metarange, and return any
		// errors.
		if err := sm.updateMetarange(rd, leftRD, rightRD); err != nil {
			return nil, err
		}
	}
	return &rfpb.SplitResponse{
		Left:  leftRD,
		Right: rightRD,
	}, nil
}

func (sm *Replica) scan(db ReplicaReader, req *rfpb.ScanRequest) (*rfpb.ScanResponse, error) {
	if len(req.GetLeft()) == 0 {
		return nil, status.InvalidArgumentError("Scan requires a valid key.")
	}

	iterOpts := &pebble.IterOptions{}
	if req.GetRight() != nil {
		iterOpts.UpperBound = req.GetRight()
	} else {
		iterOpts.UpperBound = keys.Key(req.GetLeft()).Next()
	}

	iter := db.NewIter(iterOpts)
	defer iter.Close()
	var t bool

	switch req.GetScanType() {
	case rfpb.ScanRequest_SEEKLT_SCAN_TYPE:
		t = iter.SeekLT(req.GetLeft())
	case rfpb.ScanRequest_SEEKGE_SCAN_TYPE:
		t = iter.SeekGE(req.GetLeft())
	default:
		t = iter.SeekGE(req.GetLeft())
	}

	rsp := &rfpb.ScanResponse{}
	for ; t; t = iter.Next() {
		if t {
			rsp.Kvs = append(rsp.Kvs, &rfpb.KV{
				Key:   iter.Key(),
				Value: iter.Value(),
			})
		}
	}
	return rsp, nil
}

func statusProto(err error) *statuspb.Status {
	s, _ := gstatus.FromError(err)
	return s.Proto()
}

func (sm *Replica) handlePropose(wb *pebble.Batch, req *rfpb.RequestUnion) *rfpb.ResponseUnion {
	rsp := &rfpb.ResponseUnion{}

	switch value := req.Value.(type) {
	case *rfpb.RequestUnion_FileWrite:
		r, err := sm.fileWrite(wb, value.FileWrite)
		rsp.Value = &rfpb.ResponseUnion_FileWrite{
			FileWrite: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_DirectWrite:
		r, err := sm.directWrite(wb, value.DirectWrite)
		rsp.Value = &rfpb.ResponseUnion_DirectWrite{
			DirectWrite: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_Increment:
		r, err := sm.increment(wb, value.Increment)
		rsp.Value = &rfpb.ResponseUnion_Increment{
			Increment: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_Cas:
		r, err := sm.cas(wb, value.Cas)
		rsp.Value = &rfpb.ResponseUnion_Cas{
			Cas: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_FindSplitPoint:
		r, err := sm.findSplitPoint(wb, value.FindSplitPoint)
		rsp.Value = &rfpb.ResponseUnion_FindSplitPoint{
			FindSplitPoint: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_Split:
		r, err := sm.split(wb, value.Split)
		rsp.Value = &rfpb.ResponseUnion_Split{
			Split: r,
		}
		rsp.Status = statusProto(err)

	default:
		rsp.Status = statusProto(status.UnimplementedErrorf("SyncPropose handling for %+v not implemented.", req))
	}
	return rsp
}

func (sm *Replica) handleRead(db ReplicaReader, req *rfpb.RequestUnion) *rfpb.ResponseUnion {
	rsp := &rfpb.ResponseUnion{}

	switch value := req.Value.(type) {
	case *rfpb.RequestUnion_DirectRead:
		r, err := sm.directRead(db, value.DirectRead)
		rsp.Value = &rfpb.ResponseUnion_DirectRead{
			DirectRead: r,
		}
		rsp.Status = statusProto(err)
	case *rfpb.RequestUnion_Scan:
		r, err := sm.scan(db, value.Scan)
		rsp.Value = &rfpb.ResponseUnion_Scan{
			Scan: r,
		}
		rsp.Status = statusProto(err)
	default:
		rsp.Status = statusProto(status.UnimplementedErrorf("Read handling for %+v not implemented.", req))
	}
	return rsp
}

// Update updates the IOnDiskStateMachine instance. The input Entry slice
// is a list of continuous proposed and committed commands from clients, they
// are provided together as a batch so the IOnDiskStateMachine implementation
// can choose to batch them and apply together to hide latency. Update returns
// the input entry slice with the Result field of all its members set.
//
// The read only Index field of each input Entry instance is the Raft log
// index of each entry, it is IOnDiskStateMachine's responsibility to
// atomically persist the Index value together with the corresponding state
// update.
//
// The Update method can choose to synchronize all of its in-core state with
// that on disk. This can minimize the number of committed Raft entries that
// need to be re-applied after reboot. Update can also choose to postpone such
// synchronization until the Sync method is invoked, this approach produces
// higher throughput during fault free running at the cost that some of the
// most recent Raft entries not synchronized onto disks will have to be
// re-applied after reboot.
//
// When the Update method does not synchronize its in-core state with that on
// disk, the implementation must ensure that after a reboot there is no
// applied entry in the State Machine more recent than any entry that was
// lost during reboot. For example, consider a state machine with 3 applied
// entries, let's assume their index values to be 1, 2 and 3. Once they have
// been applied into the state machine without synchronizing the in-core state
// with that on disk, it is okay to lose the data associated with the applied
// entry 3, but it is strictly forbidden to have the data associated with the
// applied entry 3 available in the state machine while the one with index
// value 2 got lost during reboot.
//
// The Update method must be deterministic, meaning given the same initial
// state of IOnDiskStateMachine and the same input sequence, it should reach
// to the same updated state and outputs the same results. The input entry
// slice should be the only input to this method. Reading from the system
// clock, random number generator or other similar external data sources will
// likely violate the deterministic requirement of the Update method.
//
// Concurrent calls to the Lookup method and the SaveSnapshot method are not
// blocked when the state machine is being updated by the Update method.
//
// The IOnDiskStateMachine implementation should not keep a reference to the
// input entry slice after return.
//
// Update returns an error when there is unrecoverable error when updating the
// on disk state machine.
func (sm *Replica) Update(entries []dbsm.Entry) ([]dbsm.Entry, error) {
	db, err := sm.DB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	wb := sm.db.NewIndexedBatch()
	defer wb.Close()

	// Insert all of the data in the batch.
	batchCmdReq := &rfpb.BatchCmdRequest{}
	for idx, entry := range entries {
		if err := proto.Unmarshal(entry.Cmd, batchCmdReq); err != nil {
			return nil, err
		}
		batchCmdRsp := &rfpb.BatchCmdResponse{}
		for _, union := range batchCmdReq.GetUnion() {
			// sm.log.Debugf("Update: request union: %+v", union)
			rsp := sm.handlePropose(wb, union)
			// sm.log.Debugf("Update: response union: %+v", rsp)
			batchCmdRsp.Union = append(batchCmdRsp.Union, rsp)
		}

		rspBuf, err := proto.Marshal(batchCmdRsp)
		if err != nil {
			return nil, err
		}
		entries[idx].Result = dbsm.Result{
			Value: uint64(len(entries[idx].Cmd)),
			Data:  rspBuf,
		}
	}
	// Also make sure to update the last applied index.
	lastEntry := entries[len(entries)-1]
	appliedIndex := uint64ToBytes(lastEntry.Index)
	if err := wb.Set(constants.LastAppliedIndexKey, appliedIndex, nil /*ignored write options*/); err != nil {
		return nil, err
	}

	if err := db.Apply(wb, &pebble.WriteOptions{Sync: true}); err != nil {
		return nil, err
	}
	if sm.lastAppliedIndex >= lastEntry.Index {
		return nil, status.FailedPreconditionError("lastApplied not moving forward")
	}
	sm.lastAppliedIndex = lastEntry.Index
	return entries, nil
}

// Lookup queries the state of the IOnDiskStateMachine instance and returns
// the query result as an interface{}. The input interface{} specifies what to
// query, it is up to the IOnDiskStateMachine implementation to interpret such
// input. The returned interface{} contains the query result.
//
// When an error is returned by the Lookup method, the error will be passed
// to the caller to be handled. A typical scenario for returning an error is
// that the state machine has already been closed or aborted from a
// RecoverFromSnapshot procedure before Lookup is called.
//
// Concurrent calls to the Update and RecoverFromSnapshot method are not
// blocked when calls to the Lookup method are being processed.
//
// The IOnDiskStateMachine implementation should not keep any reference of
// the input interface{} after return.
//
// The Lookup method is a read only method, it should never change the state
// of IOnDiskStateMachine.
func (sm *Replica) Lookup(key interface{}) (interface{}, error) {
	reqBuf, ok := key.([]byte)
	if !ok {
		return nil, status.FailedPreconditionError("Cannot convert key to []byte")
	}

	db, err := sm.Reader()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	batchCmdReq := &rfpb.BatchCmdRequest{}
	if err := proto.Unmarshal(reqBuf, batchCmdReq); err != nil {
		return nil, err
	}
	batchCmdRsp := &rfpb.BatchCmdResponse{}
	for _, req := range batchCmdReq.GetUnion() {
		//sm.log.Debugf("Lookup: request union: %+v", req)
		rsp := sm.handleRead(db, req)
		//sm.log.Debugf("Lookup: response union: %+v", rsp)
		batchCmdRsp.Union = append(batchCmdRsp.Union, rsp)
	}

	rspBuf, err := proto.Marshal(batchCmdRsp)
	if err != nil {
		return nil, err
	}
	return rspBuf, nil
}

// Sync synchronizes all in-core state of the state machine to persisted
// storage so the state machine can continue from its latest state after
// reboot.
//
// Sync is always invoked with mutual exclusion protection from the Update,
// PrepareSnapshot, RecoverFromSnapshot and Close methods.
//
// Sync returns an error when there is unrecoverable error for synchronizing
// the in-core state.
func (sm *Replica) Sync() error {
	return nil
}

// PrepareSnapshot prepares the snapshot to be concurrently captured and
// streamed. PrepareSnapshot is invoked before SaveSnapshot is called and it
// is always invoked with mutual exclusion protection from the Update, Sync,
// RecoverFromSnapshot and Close methods.
//
// PrepareSnapshot in general saves a state identifier of the current state,
// such state identifier can be a version number, a sequence number, a change
// ID or some other small in memory data structure used for describing the
// point in time state of the state machine. The state identifier is returned
// as an interface{} before being passed to the SaveSnapshot() method.
//
// PrepareSnapshot returns an error when there is unrecoverable error for
// preparing the snapshot.
func (sm *Replica) PrepareSnapshot() (interface{}, error) {
	db, err := sm.DB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	snap := db.NewSnapshot()
	return snap, nil
}

func SaveSnapshotToWriter(w io.Writer, snap *pebble.Snapshot) error {
	iter := snap.NewIter(&pebble.IterOptions{})
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		kv := &rfpb.KV{}
		kv.Key = iter.Key()
		kv.Value = iter.Value()
		protoBytes, err := proto.Marshal(kv)
		if err != nil {
			return err
		}
		msgLength := int64(len(protoBytes))
		varintBuf := make([]byte, binary.MaxVarintLen64)
		varintSize := binary.PutVarint(varintBuf, msgLength)

		// Write a header-chunk to know how big the proto is
		if _, err := w.Write(varintBuf[:varintSize]); err != nil {
			return err
		}

		// Then write the proto itself.
		n, err := w.Write(protoBytes)
		if err != nil {
			return err
		}
		if int64(n) != msgLength {
			return status.FailedPreconditionErrorf("wrote wrong number of bytes?")
		}
	}
	return nil
}

func ApplySnapshotFromReader(r io.Reader, db ReplicaWriter) error {
	wb := db.NewBatch()
	defer wb.Close()

	// Delete everything in the current database first.
	if err := wb.DeleteRange(keys.Key{constants.MinByte}, keys.Key{constants.MaxByte}, nil /*ignored write options*/); err != nil {
		return err
	}

	readBuf := bufio.NewReader(r)
	for {
		count, err := binary.ReadVarint(readBuf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		protoBytes := make([]byte, count)
		n, err := io.ReadFull(readBuf, protoBytes)
		if err != nil {
			return err
		}
		if int64(n) != count {
			return status.FailedPreconditionErrorf("Count %d != bytes read %d", count, n)
		}
		kv := &rfpb.KV{}
		if err := proto.Unmarshal(protoBytes, kv); err != nil {
			return err
		}
		if err := wb.Set(kv.Key, kv.Value, nil /*ignored write options*/); err != nil {
			return err
		}
	}
	if err := db.Apply(wb, &pebble.WriteOptions{Sync: true}); err != nil {
		return err
	}
	return nil
}

// SaveSnapshot saves the point in time state of the IOnDiskStateMachine
// instance identified by the input state identifier, which is usually not
// the latest state of the IOnDiskStateMachine instance, to the provided
// io.Writer.
//
// It is application's responsibility to save the complete state to the
// provided io.Writer in a deterministic manner. That is for the same state
// machine state, when SaveSnapshot is invoked multiple times with the same
// input state identifier, the content written to the provided io.Writer
// should always be the same.
//
// When there is any connectivity error between the local node and the remote
// node, an ErrSnapshotStreaming will be returned by io.Writer's Write method.
// The SaveSnapshot method should return ErrSnapshotStreaming to abort its
// operation.
//
// It is SaveSnapshot's responsibility to free the resources owned by the
// input state identifier when it is done.
//
// The provided read-only chan struct{} is provided to notify the SaveSnapshot
// method that the associated Raft node is being closed so the
// IOnDiskStateMachine can choose to abort the SaveSnapshot procedure and
// return ErrSnapshotStopped immediately.
//
// SaveSnapshot is allowed to abort the snapshotting operation at any time by
// returning ErrSnapshotAborted.
//
// The SaveSnapshot method is allowed to be invoked when there is concurrent
// call to the Update method. SaveSnapshot is a read-only method, it should
// never change the state of the IOnDiskStateMachine.
//
// SaveSnapshot returns the encountered error when generating the snapshot.
// Other than the above mentioned ErrSnapshotStopped and ErrSnapshotAborted
// errors, the IOnDiskStateMachine implementation should only return a non-nil
// error when the system need to be immediately halted for critical errors,
// e.g. disk error preventing you from saving the snapshot.
func (sm *Replica) SaveSnapshot(preparedSnap interface{}, w io.Writer, quit <-chan struct{}) error {
	snap, ok := preparedSnap.(*pebble.Snapshot)
	if !ok {
		return status.FailedPreconditionError("unable to coerce snapshot to *pebble.Snapshot")
	}
	defer snap.Close()
	return SaveSnapshotToWriter(w, snap)
}

// RecoverFromSnapshot recovers the state of the IOnDiskStateMachine instance
// from a snapshot captured by the SaveSnapshot() method on a remote node. The
// saved snapshot is provided as an io.Reader backed by a file stored on disk.
//
// Dragonboat ensures that the Update, Sync, PrepareSnapshot, SaveSnapshot and
// Close methods will not be invoked when RecoverFromSnapshot() is in
// progress.
//
// The provided read-only chan struct{} is provided to notify the
// RecoverFromSnapshot method that the associated Raft node has been closed.
// On receiving such notification, RecoverFromSnapshot() can choose to
// abort recovering from the snapshot and return an ErrSnapshotStopped error
// immediately. Other than ErrSnapshotStopped, IOnDiskStateMachine should
// only return a non-nil error when the system need to be immediately halted
// for non-recoverable error.
//
// RecoverFromSnapshot is not required to synchronize its recovered in-core
// state with that on disk.
func (sm *Replica) RecoverFromSnapshot(r io.Reader, quit <-chan struct{}) error {
	db, err := sm.DB()
	if err != nil {
		return err
	}
	err = ApplySnapshotFromReader(r, db)
	db.Close() // close the DB before handling errors or checking keys.
	if err != nil {
		return err
	}

	readDB, err := sm.Reader()
	if err != nil {
		return err
	}
	defer readDB.Close()
	newLastApplied, err := sm.getLastAppliedIndex(readDB)
	if err != nil {
		return err
	}
	if sm.lastAppliedIndex > newLastApplied {
		return status.FailedPreconditionErrorf("last applied not moving forward: %d > %d", sm.lastAppliedIndex, newLastApplied)
	}
	sm.lastAppliedIndex = newLastApplied
	sm.checkAndSetRangeDescriptor(readDB)
	return nil
}

// Close closes the IOnDiskStateMachine instance. Close is invoked when the
// state machine is in a ready-to-exit state in which there will be no further
// call to the Update, Sync, PrepareSnapshot, SaveSnapshot and the
// RecoverFromSnapshot method. It is possible to have concurrent Lookup calls,
// Lookup can also be called after the return of Close.
//
// Close allows the application to finalize resources to a state easier to
// be re-opened and restarted in the future. It is important to understand
// that Close is not guaranteed to be always invoked, e.g. node can crash at
// any time without calling the Close method. IOnDiskStateMachine should be
// designed in a way that the safety and integrity of its on disk data
// doesn't rely on whether Close is eventually called or not.
//
// Other than setting up some internal flags to indicate that the
// IOnDiskStateMachine instance has been closed, the Close method is not
// allowed to update the state of IOnDiskStateMachine visible to the outside.
func (sm *Replica) Close() error {
	sm.closedMu.Lock()
	defer sm.closedMu.Unlock()
	if sm.closed {
		return nil
	}
	sm.closed = true
	sm.wg.Wait() // wait for all db users to finish up.

	sm.rangeMu.Lock()
	rangeDescriptor := sm.rangeDescriptor
	sm.rangeMu.Unlock()

	if sm.store != nil && rangeDescriptor != nil {
		sm.store.RemoveRange(rangeDescriptor, sm)
	}
	if sm.db != nil {
		return sm.db.Close()
	}
	return nil
}

// CreateReplica creates an ondisk statemachine.
func New(rootDir, fileDir string, clusterID, nodeID uint64, store IStore) *Replica {
	return &Replica{
		closedMu:  &sync.RWMutex{},
		wg:        sync.WaitGroup{},
		rootDir:   rootDir,
		fileDir:   fileDir,
		clusterID: clusterID,
		nodeID:    nodeID,
		store:     store,
		log:       log.NamedSubLogger(fmt.Sprintf("c%dn%d", clusterID, nodeID)),
	}
}
