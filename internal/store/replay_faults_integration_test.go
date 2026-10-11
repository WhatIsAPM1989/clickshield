package store

import (
	"context"
	"errors"
	"github.com/lamooof/clickshield/internal/replay"
	"testing"
	"time"
)

type failingObjects struct {
	replay.Objects
	failPut, failDelete bool
	deleteKey           string
	beforePut           func()
}

func (o *failingObjects) Put(ctx context.Context, key string, data []byte) error {
	if o.failPut {
		o.failPut = false
		return errors.New("test upload unavailable")
	}
	if o.beforePut != nil {
		o.beforePut()
		o.beforePut = nil
	}
	return o.Objects.Put(ctx, key, data)
}
func (o *failingObjects) Delete(ctx context.Context, key string) error {
	if o.failDelete && key == o.deleteKey {
		o.failDelete = false
		return errors.New("test delete unavailable")
	}
	return o.Objects.Delete(ctx, key)
}

func TestReplayPendingRetryAndDurableObjectDeletion(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	if err := st.UpdateReplaySettings(ctx, p.ID, true, 100, 7); err != nil {
		t.Fatal(err)
	}
	objects := &failingObjects{Objects: testReplayObjects(t), failPut: true}
	repo := replay.NewRepository(st.Pool)
	recorder := replay.NewService(repo, objects)
	page, _ := UUID()
	now := time.Now().UnixMilli()
	part := replay.Part{SessionID: b.SessionID, VisitorID: b.VisitorID, PageID: page, PageIndex: 1, PageURL: b.PageURL, PartCount: 1, StartedAt: now, EndedAt: now, Data: []byte(`[{"type":4,"timestamp":1,"data":{}}]`)}
	if err := recorder.SaveReplayPart(ctx, p.ID, part); err == nil {
		t.Fatal("upload should fail")
	}
	var ready bool
	var size int64
	if err := st.Pool.QueryRow(ctx, `SELECT ready FROM replay_chunks WHERE recording_id=$1`, b.SessionID).Scan(&ready); err != nil || ready {
		t.Fatal("failed upload became ready", err)
	}
	if err := recorder.SaveReplayPart(ctx, p.ID, part); err != nil {
		t.Fatal(err)
	}
	if err := recorder.SaveReplayPart(ctx, p.ID, part); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `SELECT total_bytes FROM replay_recordings WHERE id=$1`, b.SessionID).Scan(&size); err != nil || size != int64(len(part.Data)) {
		t.Fatalf("retry double-counted quota: %d %v", size, err)
	}
	data, err := recorder.Replay(ctx, b.SessionID)
	if err != nil || data.Incomplete {
		t.Fatal("pending upload did not recover", err)
	}
	if err = st.Pool.QueryRow(ctx, `SELECT object_key FROM replay_chunks WHERE recording_id=$1`, b.SessionID).Scan(&objects.deleteKey); err != nil {
		t.Fatal(err)
	}
	if err = recorder.Delete(ctx, b.SessionID); err != nil {
		t.Fatal(err)
	}
	objects.failDelete = true
	if _, err = st.Pool.Exec(ctx, `UPDATE replay_object_gc SET not_before=now() WHERE object_key=$1`, objects.deleteKey); err != nil {
		t.Fatal(err)
	}
	var failed bool
	for i := 0; i < 1000; i++ {
		ok, err := repo.CollectOne(ctx, objects)
		if err != nil {
			failed = true
			break
		}
		if !ok {
			break
		}
	}
	if !failed {
		t.Fatal("expected object deletion failure")
	}
	var attempts int
	if err = st.Pool.QueryRow(ctx, `SELECT attempts FROM replay_object_gc WHERE object_key=$1`, objects.deleteKey).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("failed GC lost retry", err)
	}
	st.Pool.Exec(ctx, `UPDATE replay_object_gc SET not_before=now() WHERE object_key=$1`, objects.deleteKey)
	for i := 0; i < 1000; i++ {
		ok, err := repo.CollectOne(ctx, objects)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
	}
	if _, err = objects.Get(ctx, objects.deleteKey); err == nil {
		t.Fatal("deleted object remains in S3")
	}
	if err = recorder.SaveReplayPart(ctx, p.ID, part); !errors.Is(err, replay.ErrNotFound) {
		t.Fatal("privacy deletion resurrected replay", err)
	}
}

func TestReplayDeleteRacingUploadIsCollected(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	if err := st.UpdateReplaySettings(ctx, p.ID, true, 100, 7); err != nil {
		t.Fatal(err)
	}
	repo := replay.NewRepository(st.Pool)
	objects := &failingObjects{Objects: testReplayObjects(t), beforePut: func() {
		if err := repo.Delete(ctx, b.SessionID); err != nil {
			t.Fatal(err)
		}
	}}
	page, _ := UUID()
	now := time.Now().UnixMilli()
	part := replay.Part{SessionID: b.SessionID, VisitorID: b.VisitorID, PageID: page, PageIndex: 1, PageURL: b.PageURL, PartCount: 1, StartedAt: now, EndedAt: now, Data: []byte(`[]`)}
	if err := replay.NewService(repo, objects).SaveReplayPart(ctx, p.ID, part); !errors.Is(err, replay.ErrNotFound) {
		t.Fatal("late upload became visible", err)
	}
	var key string
	if err := st.Pool.QueryRow(ctx, `SELECT object_key FROM replay_object_gc WHERE object_key LIKE $1`, "%"+b.SessionID+"%").Scan(&key); err != nil {
		t.Fatal("racing upload lost deletion notification", err)
	}
	st.Pool.Exec(ctx, `UPDATE replay_object_gc SET not_before=now() WHERE object_key=$1`, key)
	for i := 0; i < 1000; i++ {
		ok, err := repo.CollectOne(ctx, objects)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
	}
	if _, err := objects.Get(ctx, key); err == nil {
		t.Fatal("racing upload leaked object")
	}
}
