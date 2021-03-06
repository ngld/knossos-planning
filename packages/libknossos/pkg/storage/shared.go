package storage

import (
	"context"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/ngld/knossos/packages/libknossos/pkg/api"
)

type txCtxKey struct{}

var db *bolt.DB

func Open(ctx context.Context) error {
	var err error

	dbPath := filepath.Join(api.SettingsPath(ctx), "state.db")
	newDB, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return err
	}

	buckets := [][]byte{
		localModsBucket, remoteModsBucket, indexBucket, fileBucket, settingsBucket, userModSettingsBucket,
		engineFlagsBucket, httpCacheBucket,
	}
	err = newDB.Update(func(tx *bolt.Tx) error {
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	db = newDB

	return db.View(func(tx *bolt.Tx) error {
		err := localVersionIdx.Open(tx)
		if err != nil {
			return err
		}

		err = localTypeIdx.Open(tx)
		if err != nil {
			return err
		}

		err = remoteVersionIdx.Open(tx)
		if err != nil {
			return err
		}

		return remoteTypeIdx.Open(tx)
	})
}

func Clean(ctx context.Context) error {
	// TODO This should delete all unknown keys and if possible unreferenced entries from the DB
	return db.Update(func(tx *bolt.Tx) error {
		return cleanEngineFlags(ctx, tx)
	})
}

func Close(ctx context.Context) {
	db.Close()
	db = nil
}

func CtxWithTx(ctx context.Context, tx *bolt.Tx) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

func TxFromCtx(ctx context.Context) *bolt.Tx {
	val := ctx.Value(txCtxKey{})
	if val == nil {
		return nil
	}
	return val.(*bolt.Tx)
}

func view(ctx context.Context, cb func(*bolt.Tx) error) error {
	tx := TxFromCtx(ctx)
	if tx != nil {
		return cb(tx)
	}

	return db.View(cb)
}

func update(ctx context.Context, cb func(*bolt.Tx) error) error {
	tx := TxFromCtx(ctx)
	if tx != nil {
		return cb(tx)
	}

	return db.Update(cb)
}

func BatchUpdate(ctx context.Context, callback func(context.Context) error) error {
	return db.Batch(func(tx *bolt.Tx) error {
		return callback(CtxWithTx(ctx, tx))
	})
}

func BatchRead(ctx context.Context, callback func(context.Context) error) error {
	return db.View(func(tx *bolt.Tx) error {
		return callback(CtxWithTx(ctx, tx))
	})
}
