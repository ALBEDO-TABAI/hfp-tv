package workshop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// BlobStore keeps workshop files by SHA-256 under <root>/community/blobs.
// Files are written once and never modified; the hash is computed while
// writing, so a stored name always matches its content.
type BlobStore struct{ dir string }

func NewBlobStore(dataRoot string) (*BlobStore, error) {
	dir := filepath.Join(dataRoot, "community", "blobs", "sha256")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &BlobStore{dir: dir}, nil
}

func (b *BlobStore) path(sha string) string {
	return filepath.Join(b.dir, sha[:2], sha)
}

func validSHA(sha string) bool {
	if len(sha) != 64 {
		return false
	}
	_, err := hex.DecodeString(sha)
	return err == nil
}

func (b *BlobStore) Put(body io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(b.dir, ".upload-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), body)
	if err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	sha := hex.EncodeToString(hash.Sum(nil))
	target := b.path(sha)
	if _, err := os.Stat(target); err == nil {
		return sha, size, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", 0, err
	}
	return sha, size, os.Rename(tmp.Name(), target)
}

var ErrBlobMissing = errors.New("工坊文件不存在")

func (b *BlobStore) Open(sha string) (io.ReadCloser, error) {
	if !validSHA(sha) {
		return nil, ErrBlobMissing
	}
	file, err := os.Open(b.path(sha))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrBlobMissing
	}
	return file, err
}

// FilePath lets HTTP handlers serve ranges directly from disk.
func (b *BlobStore) FilePath(sha string) (string, error) {
	if !validSHA(sha) {
		return "", ErrBlobMissing
	}
	path := b.path(sha)
	if _, err := os.Stat(path); err != nil {
		return "", ErrBlobMissing
	}
	return path, nil
}
