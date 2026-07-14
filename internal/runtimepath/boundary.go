package runtimepath

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Boundary binds later descriptor-relative operations to the repository root
// inode that was validated when the boundary was opened. It contains no open
// descriptor and is safe to retain for the lifetime of an owner.
type Boundary struct {
	root     string
	identity fileIdentity
}

// Directory is one opened, identity-checked directory below a Boundary.
// Metadata operations on it are descriptor-relative and cannot be redirected
// by replacing an ancestor pathname.
type Directory struct {
	boundary Boundary
	path     string
	file     *os.File
	identity fileIdentity
	closed   bool
}

// File is one opened, identity-checked protected regular file. Its parent and
// opened inode stay bound across publication and cleanup operations.
type File struct {
	directory *Directory
	name      string
	file      *os.File
	identity  fileIdentity
	closed    bool
	removed   bool
}

type fileIdentity struct {
	device uint64
	inode  uint64
}

// Bind resolves and validates a repository root once and remembers its inode.
// Later operations fail if the named root is replaced.
func Bind(root string) (Boundary, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Boundary{}, err
	}
	fd, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ELOOP) {
		return Boundary{}, unsafe(canonical, canonical, "became a symlink during root open")
	}
	if err != nil {
		return Boundary{}, err
	}
	file := os.NewFile(uintptr(fd), canonical)
	if file == nil {
		_ = unix.Close(fd)
		return Boundary{}, errors.New("harness runtime path: open repository root descriptor")
	}
	defer file.Close()
	stat, err := fstat(file)
	if err != nil {
		return Boundary{}, err
	}
	if !directoryMode(stat) {
		return Boundary{}, unsafe(canonical, canonical, "repository root is not a directory")
	}
	var named unix.Stat_t
	if err := unix.Fstatat(unix.AT_FDCWD, canonical, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return Boundary{}, err
	}
	if named.Mode&unix.S_IFMT == unix.S_IFLNK || identityOf(&named) != identityOf(stat) {
		return Boundary{}, unsafe(canonical, canonical, "opened root does not match the named directory")
	}
	return Boundary{root: canonical, identity: identityOf(stat)}, nil
}

func (b Boundary) Root() string { return b.root }

func (b Boundary) CheckDir(target string, missingOK bool) error {
	dir, found, err := b.OpenDir(target, missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			return os.ErrNotExist
		}
		return err
	}
	return dir.Close()
}

func (b Boundary) CheckFile(path string, missingOK bool) error {
	dir, found, err := b.OpenDir(filepath.Dir(path), missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			return os.ErrNotExist
		}
		return err
	}
	defer dir.Close()
	stat, found, err := dir.namedFileStat(filepath.Base(path))
	if err != nil {
		return err
	}
	if !found {
		if missingOK {
			return nil
		}
		return os.ErrNotExist
	}
	return checkRegularFileStat(b.root, path, stat)
}

func (b Boundary) ReadFile(path string, missingOK bool) ([]byte, bool, error) {
	dir, found, err := b.OpenDir(filepath.Dir(path), missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			err = os.ErrNotExist
		}
		return nil, false, err
	}
	defer dir.Close()
	return dir.ReadFile(filepath.Base(path), missingOK)
}

func (b Boundary) ReadDir(path string, missingOK bool) ([]os.DirEntry, bool, error) {
	dir, found, err := b.OpenDir(path, missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			err = os.ErrNotExist
		}
		return nil, false, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	return entries, true, err
}

func (b Boundary) SyncDir(path string) error {
	dir, found, err := b.OpenDir(path, false)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// EnsureDir creates missing directory components relative to stable opened
// ancestors and validates every component.
func (b Boundary) EnsureDir(target string, mode os.FileMode) error {
	dir, _, err := b.openDir(target, true, mode)
	if err != nil {
		return err
	}
	return dir.Close()
}

// OpenDir opens an existing protected directory. An absent path is reported
// with found=false when missingOK is true.
func (b Boundary) OpenDir(target string, missingOK bool) (*Directory, bool, error) {
	dir, found, err := b.openDir(target, false, 0)
	if err != nil || found || missingOK {
		return dir, found, err
	}
	return nil, false, os.ErrNotExist
}

func (b Boundary) openDir(target string, create bool, mode os.FileMode) (*Directory, bool, error) {
	parts, err := relativeParts(b.root, target)
	if err != nil {
		return nil, false, err
	}
	current, err := b.openRoot()
	if err != nil {
		return nil, false, err
	}
	currentPath := b.root
	for _, part := range parts {
		childPath := filepath.Join(currentPath, part)
		child, stat, openErr := openDirectoryAt(current, part, b.root, childPath)
		if errors.Is(openErr, os.ErrNotExist) && create {
			if mkdirErr := unix.Mkdirat(int(current.Fd()), part, uint32(mode.Perm())); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = current.Close()
				return nil, false, mkdirErr
			}
			child, stat, openErr = openDirectoryAt(current, part, b.root, childPath)
		}
		if errors.Is(openErr, os.ErrNotExist) {
			_ = current.Close()
			return nil, false, nil
		}
		if openErr != nil {
			_ = current.Close()
			return nil, false, openErr
		}
		_ = current.Close()
		current = child
		currentPath = childPath
		if err := checkDirectoryStat(b.root, currentPath, stat); err != nil {
			_ = current.Close()
			return nil, false, err
		}
	}
	stat, err := fstat(current)
	if err != nil {
		_ = current.Close()
		return nil, false, err
	}
	return &Directory{boundary: b, path: filepath.Clean(target), file: current, identity: identityOf(stat)}, true, nil
}

func (b Boundary) openRoot() (*os.File, error) {
	if strings.TrimSpace(b.root) == "" {
		return nil, errors.New("harness runtime path: uninitialized boundary")
	}
	fd, err := unix.Open(b.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ELOOP) {
		return nil, unsafe(b.root, b.root, "became a symlink during root open")
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), b.root)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("harness runtime path: open repository root descriptor")
	}
	stat, err := fstat(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !directoryMode(stat) || identityOf(stat) != b.identity {
		_ = file.Close()
		return nil, unsafe(b.root, b.root, "repository root identity changed")
	}
	return file, nil
}

// Check proves that the directory still occupies its canonical namespace.
func (d *Directory) Check() error {
	if d == nil || d.closed || d.file == nil {
		return errors.New("harness runtime path: directory is closed")
	}
	opened, found, err := d.boundary.OpenDir(d.path, true)
	if err != nil {
		return err
	}
	if !found {
		return unsafe(d.boundary.root, d.path, "opened directory is no longer named")
	}
	defer opened.Close()
	if opened.identity != d.identity {
		return unsafe(d.boundary.root, d.path, "opened directory does not match the named component")
	}
	stat, err := fstat(d.file)
	if err != nil {
		return err
	}
	if identityOf(stat) != d.identity {
		return unsafe(d.boundary.root, d.path, "opened directory identity changed")
	}
	if d.identity == d.boundary.identity && filepath.Clean(d.path) == d.boundary.root {
		if !directoryMode(stat) {
			return unsafe(d.boundary.root, d.path, "repository root is not a directory")
		}
		return nil
	}
	return checkDirectoryStat(d.boundary.root, d.path, stat)
}

func (d *Directory) Close() error {
	if d == nil || d.closed {
		return nil
	}
	d.closed = true
	return d.file.Close()
}

// OpenFile opens a protected regular file relative to the stable directory.
func (d *Directory) OpenFile(name string, flag int, perm os.FileMode) (*File, error) {
	if err := finalName(name); err != nil {
		return nil, unsafe(d.boundary.root, filepath.Join(d.path, name), err.Error())
	}
	if flag&os.O_TRUNC != 0 {
		return nil, unsafe(d.boundary.root, filepath.Join(d.path, name), "cannot be opened with truncation before identity validation")
	}
	if err := d.Check(); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, flag|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, uint32(perm.Perm()))
	if errors.Is(err, unix.ELOOP) {
		return nil, unsafe(d.boundary.root, filepath.Join(d.path, name), "became a symlink during open")
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filepath.Join(d.path, name))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("harness runtime path: open file descriptor")
	}
	stat, err := fstat(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	path := filepath.Join(d.path, name)
	if err := checkRegularFileStat(d.boundary.root, path, stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	result := &File{directory: d, name: name, file: file, identity: identityOf(stat)}
	if err := result.Check(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return result, nil
}

func (d *Directory) CreateTemp(prefix string, perm os.FileMode) (*File, error) {
	if prefix == "" || strings.ContainsAny(prefix, `/\\`) {
		return nil, errors.New("harness runtime path: safe temporary prefix is required")
	}
	var random [16]byte
	for range 100 {
		if _, err := rand.Read(random[:]); err != nil {
			return nil, err
		}
		name := prefix + hex.EncodeToString(random[:])
		file, err := d.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("harness runtime path: exhausted temporary file names")
}

// ReadFile reads through an opened protected file and rechecks both the file
// and directory identities before returning bytes.
func (d *Directory) ReadFile(name string, missingOK bool) ([]byte, bool, error) {
	file, err := d.OpenFile(name, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	raw, err := file.ReadAll()
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// ReadDir enumerates through a fresh descriptor for this stable directory.
func (d *Directory) ReadDir() ([]os.DirEntry, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	opened, stat, err := openDirectoryAt(d.file, ".", d.boundary.root, d.path)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	if identityOf(stat) != d.identity {
		return nil, unsafe(d.boundary.root, d.path, "enumerated directory identity changed")
	}
	entries, err := opened.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	if err := d.Check(); err != nil {
		return nil, err
	}
	return entries, nil
}

// Replace atomically publishes one opened temporary file under destName.
func (d *Directory) Replace(temp *File, destName string) error {
	if err := finalName(destName); err != nil {
		return unsafe(d.boundary.root, filepath.Join(d.path, destName), err.Error())
	}
	if temp == nil || temp.directory != d || temp.closed || temp.removed {
		return errors.New("harness runtime path: replacement file is not owned by the directory")
	}
	if err := d.Check(); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if stat, found, err := d.namedFileStat(destName); err != nil {
		return err
	} else if found {
		if err := checkRegularFileStat(d.boundary.root, filepath.Join(d.path, destName), stat); err != nil {
			return err
		}
	}
	if err := unix.Renameat(int(d.file.Fd()), temp.name, int(d.file.Fd()), destName); err != nil {
		return err
	}
	temp.name = destName
	if err := d.Check(); err != nil {
		return err
	}
	return temp.Check()
}

// Link publishes one opened temporary file without replacing an existing
// destination, then removes the temporary name. The opened inode remains bound
// throughout the two descriptor-relative metadata operations.
func (d *Directory) Link(temp *File, destName string) error {
	if err := finalName(destName); err != nil {
		return unsafe(d.boundary.root, filepath.Join(d.path, destName), err.Error())
	}
	if temp == nil || temp.directory != d || temp.closed || temp.removed {
		return errors.New("harness runtime path: link source is not owned by the directory")
	}
	if err := d.Check(); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if _, found, err := d.namedFileStat(destName); err != nil {
		return err
	} else if found {
		return os.ErrExist
	}
	if err := unix.Linkat(int(d.file.Fd()), temp.name, int(d.file.Fd()), destName, 0); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(d.file.Fd()), temp.name, 0); err != nil {
		return err
	}
	temp.name = destName
	if err := d.Check(); err != nil {
		return err
	}
	return temp.Check()
}

// Remove unlinks the still-open file only if its stable parent and named inode
// remain unchanged.
func (d *Directory) Remove(file *File) error {
	if file == nil || file.directory != d || file.closed || file.removed {
		return errors.New("harness runtime path: removable file is not owned by the directory")
	}
	if err := d.Check(); err != nil {
		return err
	}
	if err := file.Check(); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(d.file.Fd()), file.name, 0); err != nil {
		return err
	}
	file.removed = true
	return d.Check()
}

func (d *Directory) Sync() error {
	if err := d.Check(); err != nil {
		return err
	}
	if err := d.file.Sync(); err != nil {
		return err
	}
	return d.Check()
}

func (d *Directory) namedFileStat(name string) (*unix.Stat_t, bool, error) {
	if err := finalName(name); err != nil {
		return nil, false, unsafe(d.boundary.root, filepath.Join(d.path, name), err.Error())
	}
	var stat unix.Stat_t
	err := unix.Fstatat(int(d.file.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &stat, true, nil
}

func (f *File) Check() error {
	if f == nil || f.closed || f.file == nil || f.removed {
		return errors.New("harness runtime path: file is closed or removed")
	}
	if err := f.directory.Check(); err != nil {
		return err
	}
	stat, found, err := f.directory.namedFileStat(f.name)
	if err != nil {
		return err
	}
	if !found || identityOf(stat) != f.identity {
		return unsafe(f.directory.boundary.root, filepath.Join(f.directory.path, f.name), "opened file does not match the named component")
	}
	opened, err := fstat(f.file)
	if err != nil {
		return err
	}
	if identityOf(opened) != f.identity {
		return unsafe(f.directory.boundary.root, filepath.Join(f.directory.path, f.name), "opened file identity changed")
	}
	return checkRegularFileStat(f.directory.boundary.root, filepath.Join(f.directory.path, f.name), opened)
}

func (f *File) Write(raw []byte) (int, error) {
	if err := f.Check(); err != nil {
		return 0, err
	}
	return f.file.Write(raw)
}

func (f *File) ReadAll() ([]byte, error) {
	if err := f.Check(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(f.file)
	if err != nil {
		return nil, err
	}
	if err := f.Check(); err != nil {
		return nil, err
	}
	return raw, nil
}

func (f *File) Sync() error {
	if err := f.Check(); err != nil {
		return err
	}
	if err := f.file.Sync(); err != nil {
		return err
	}
	return f.Check()
}

func (f *File) Close() error {
	if f == nil || f.closed {
		return nil
	}
	f.closed = true
	return f.file.Close()
}

func (f *File) release() *os.File {
	file := f.file
	f.file = nil
	f.closed = true
	return file
}

func openDirectoryAt(parent *os.File, name, root, path string) (*os.File, *unix.Stat_t, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		var named unix.Stat_t
		if statErr := unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW); statErr == nil {
			return nil, nil, checkDirectoryStat(root, path, &named)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, errors.New("harness runtime path: open directory descriptor")
	}
	stat, err := fstat(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	var named unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if identityOf(stat) != identityOf(&named) {
		_ = file.Close()
		return nil, nil, unsafe(root, path, "opened directory does not match the named component")
	}
	return file, stat, nil
}

func fstat(file *os.File) (*unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return nil, err
	}
	return &stat, nil
}

func identityOf(stat *unix.Stat_t) fileIdentity {
	return fileIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}
}

func directoryMode(stat *unix.Stat_t) bool { return stat.Mode&unix.S_IFMT == unix.S_IFDIR }

func checkDirectoryStat(root, path string, stat *unix.Stat_t) error {
	switch {
	case stat.Mode&unix.S_IFMT == unix.S_IFLNK:
		return unsafe(root, path, "is a symlink")
	case !directoryMode(stat):
		return unsafe(root, path, "is not a directory")
	case os.FileMode(stat.Mode).Perm()&0o022 != 0:
		return unsafe(root, path, fmt.Sprintf("has unsafe directory mode %04o", os.FileMode(stat.Mode).Perm()))
	default:
		return nil
	}
}

func checkRegularFileStat(root, path string, stat *unix.Stat_t) error {
	switch {
	case stat.Mode&unix.S_IFMT == unix.S_IFLNK:
		return unsafe(root, path, "is a symlink")
	case stat.Mode&unix.S_IFMT != unix.S_IFREG:
		return unsafe(root, path, "is not a regular file")
	case os.FileMode(stat.Mode).Perm()&0o022 != 0:
		return unsafe(root, path, fmt.Sprintf("has unsafe file mode %04o", os.FileMode(stat.Mode).Perm()))
	case uint64(stat.Nlink) != 1:
		return unsafe(root, path, "has an unexpected hard-link count")
	default:
		return nil
	}
}

func finalName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return errors.New("safe final component is required")
	}
	return nil
}
