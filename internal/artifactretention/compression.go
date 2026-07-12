package artifactretention

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"revolvr/internal/pathguard"
)

const CompressionManifestSchema = "revolvr-compressed-artifact-v1"

type Identity struct {
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
}
type CompressionManifest struct {
	SchemaVersion  string    `json:"schema_version"`
	OriginalPath   string    `json:"original_path"`
	Original       Identity  `json:"original"`
	OriginalMTime  time.Time `json:"original_mtime"`
	CompressedPath string    `json:"compressed_path"`
	Compressed     Identity  `json:"compressed"`
}

func deterministicGzip(ctx context.Context, raw []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw, err := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	zw.Header.ModTime = time.Unix(0, 0).UTC()
	zw.Header.Name = ""
	zw.Header.Comment = ""
	zw.Header.OS = 255
	const chunk = 64 * 1024
	for start := 0; start < len(raw); start += chunk {
		if err := ctx.Err(); err != nil {
			zw.Close()
			return nil, err
		}
		end := start + chunk
		if end > len(raw) {
			end = len(raw)
		}
		if _, err := zw.Write(raw[start:end]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Read opens a logical artifact from its original representation or from an
// admitted deterministic gzip representation while preserving its original
// identity. Dual representations fail closed.
func Read(ctx context.Context, repositoryRoot, logicalPath string, expected Identity, capBytes int64) ([]byte, error) {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return nil, err
	}
	abs, err := resolveLogical(root, logicalPath)
	if err != nil {
		return nil, err
	}
	_, originalErr := os.Lstat(abs)
	manifestAbs := abs + ".gz.manifest.json"
	_, manifestErr := os.Lstat(manifestAbs)
	if originalErr == nil && manifestErr == nil {
		return nil, errors.New("compressed artifact: conflicting dual representation")
	}
	if originalErr == nil {
		raw, _, err := readRegular(abs, capBytes)
		if err != nil {
			return nil, err
		}
		if err := matchIdentity(raw, expected); err != nil {
			return nil, err
		}
		return raw, nil
	}
	if !errors.Is(originalErr, os.ErrNotExist) {
		return nil, originalErr
	}
	if manifestErr != nil {
		return nil, errors.Join(manifestErr, errors.New("compressed artifact: logical artifact is missing"))
	}
	manifestRaw, _, err := readRegular(manifestAbs, 1<<20)
	if err != nil {
		return nil, err
	}
	var manifest CompressionManifest
	if err := strictJSON(manifestRaw, &manifest); err != nil {
		return nil, err
	}
	canonical, _ := canonicalJSON(manifest)
	if !bytes.Equal(manifestRaw, canonical) {
		return nil, errors.New("compressed artifact: non-canonical manifest")
	}
	if manifest.SchemaVersion != CompressionManifestSchema || filepath.ToSlash(filepath.Clean(manifest.OriginalPath)) != filepath.ToSlash(filepath.Clean(logicalPath)) || manifest.Original != expected {
		return nil, errors.New("compressed artifact: manifest authority mismatch")
	}
	gzAbs, err := resolveLogical(root, manifest.CompressedPath)
	if err != nil {
		return nil, err
	}
	gzRaw, _, err := readRegular(gzAbs, manifest.Compressed.ByteSize)
	if err != nil {
		return nil, err
	}
	if err := matchIdentity(gzRaw, manifest.Compressed); err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(gzRaw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	if capBytes <= 0 {
		return nil, errors.New("compressed artifact: positive decompression cap required")
	}
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: zr}, capBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > capBytes {
		return nil, errors.New("compressed artifact: decompression cap exceeded")
	}
	if err := matchIdentity(raw, expected); err != nil {
		return nil, err
	}
	return raw, nil
}

// ReadLogical discovers the original identity from a compression manifest
// only when the ordinary representation is absent. It is for legacy readers
// whose ledger path predates explicit hash fields.
func ReadLogical(ctx context.Context, repositoryRoot, logicalPath string, capBytes int64) ([]byte, Identity, error) {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return nil, Identity{}, err
	}
	abs, err := resolveLogical(root, logicalPath)
	if err != nil {
		return nil, Identity{}, err
	}
	if raw, _, readErr := readRegular(abs, capBytes); readErr == nil {
		if _, manifestErr := os.Lstat(abs + ".gz.manifest.json"); manifestErr == nil {
			return nil, Identity{}, errors.New("compressed artifact: conflicting dual representation")
		}
		id := identity(raw)
		return raw, id, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return nil, Identity{}, readErr
	}
	manifestRaw, _, err := readRegular(abs+".gz.manifest.json", 1<<20)
	if err != nil {
		return nil, Identity{}, err
	}
	var manifest CompressionManifest
	if err := strictJSON(manifestRaw, &manifest); err != nil {
		return nil, Identity{}, err
	}
	raw, err := Read(ctx, root, logicalPath, manifest.Original, capBytes)
	return raw, manifest.Original, err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func compressIdentity(ctx context.Context, raw []byte) ([]byte, Identity, error) {
	gz, err := deterministicGzip(ctx, raw)
	if err != nil {
		return nil, Identity{}, err
	}
	return gz, identity(gz), nil
}
func identity(raw []byte) Identity {
	sum := sha256.Sum256(raw)
	return Identity{SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(raw))}
}
func matchIdentity(raw []byte, want Identity) error {
	if identity(raw) != want {
		return errors.New("artifact identity mismatch")
	}
	return nil
}

func readRegular(path string, capBytes int64) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("artifact is not a regular non-symlink file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		return nil, nil, errors.New("artifact has unexpected hard links")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, nil, errors.New("artifact has unsafe group/world-write mode")
	}
	if capBytes >= 0 && info.Size() > capBytes {
		return nil, nil, errors.New("artifact exceeds read cap")
	}
	raw, err := os.ReadFile(path)
	return raw, info, err
}

func resolveLogical(root, value string) (string, error) {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		abs, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		if !pathguard.WithinRoot(root, abs) {
			return "", errors.New("artifact path escapes repository")
		}
		value, _ = filepath.Rel(root, abs)
	}
	return pathguard.Resolve(root, value)
}
func canonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func strictJSON(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func writeAtomic(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-retention-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

var _ = fmt.Sprintf
