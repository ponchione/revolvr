package ledger

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"time"
)

const SnapshotIdentitySchema = "revolvr-ledger-logical-snapshot-v1"

// SnapshotIdentity is the deterministic identity of every logical value in a
// ledger snapshot. ByteSize is the size of the canonical length-prefixed
// identity stream, not the size of the physical SQLite database file.
type SnapshotIdentity struct {
	SHA256   string
	ByteSize int64
}

// IdentifySnapshot hashes the complete ordered logical snapshot. Event
// payloads are included byte-for-byte so equivalent JSON values with different
// source bytes remain distinct evidence.
func IdentifySnapshot(snapshot Snapshot) SnapshotIdentity {
	w := snapshotIdentityWriter{hash: sha256.New()}
	w.writeString(SnapshotIdentitySchema)
	w.writeInt64(snapshot.MaxEventID)
	w.writeUint64(uint64(len(snapshot.Runs)))
	for _, history := range snapshot.Runs {
		run := history.Run
		w.writeString(run.ID)
		w.writeString(run.TaskID)
		w.writeString(run.Task)
		w.writeString(run.Status)
		w.writeString(run.Summary)
		w.writeTime(run.StartedAt)
		w.writeOptionalTime(run.CompletedAt)
		w.writeInt64(int64(run.DurationSeconds))
		w.writeOptionalInt(run.CodexExitCode)
		w.writeString(run.VerificationStatus)
		w.writeString(run.CommitSHA)
		w.writeUint64(uint64(len(history.Events)))
		for _, event := range history.Events {
			w.writeInt64(event.ID)
			w.writeString(event.RunID)
			w.writeString(string(event.Type))
			w.writeOptionalBytes(event.Payload)
			w.writeTime(event.CreatedAt)
		}
	}
	return SnapshotIdentity{SHA256: hex.EncodeToString(w.hash.Sum(nil)), ByteSize: w.size}
}

type snapshotIdentityWriter struct {
	hash hash.Hash
	size int64
}

func (w *snapshotIdentityWriter) write(raw []byte) {
	_, _ = w.hash.Write(raw)
	w.size += int64(len(raw))
}

func (w *snapshotIdentityWriter) writeByte(value byte) {
	w.write([]byte{value})
}

func (w *snapshotIdentityWriter) writeUint64(value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	w.write(raw[:])
}

func (w *snapshotIdentityWriter) writeInt64(value int64) {
	w.writeUint64(uint64(value))
}

func (w *snapshotIdentityWriter) writeBytes(value []byte) {
	w.writeUint64(uint64(len(value)))
	w.write(value)
}

func (w *snapshotIdentityWriter) writeString(value string) {
	w.writeBytes([]byte(value))
}

func (w *snapshotIdentityWriter) writeTime(value time.Time) {
	w.writeString(value.UTC().Format(time.RFC3339Nano))
}

func (w *snapshotIdentityWriter) writeOptionalTime(value *time.Time) {
	if value == nil {
		w.writeByte(0)
		return
	}
	w.writeByte(1)
	w.writeTime(*value)
}

func (w *snapshotIdentityWriter) writeOptionalInt(value *int) {
	if value == nil {
		w.writeByte(0)
		return
	}
	w.writeByte(1)
	w.writeInt64(int64(*value))
}

func (w *snapshotIdentityWriter) writeOptionalBytes(value []byte) {
	if value == nil {
		w.writeByte(0)
		return
	}
	w.writeByte(1)
	w.writeBytes(value)
}
