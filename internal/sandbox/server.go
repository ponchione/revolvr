package sandbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	ProtocolSchemaVersion = "revolvr-sandboxd-protocol-v1"
	maxProtocolFrame      = 1 << 20
)

type RunRequest struct {
	SchemaVersion string        `json:"schema_version"`
	Specification Specification `json:"specification"`
}

type RunResponse struct {
	SchemaVersion string   `json:"schema_version"`
	Evidence      Evidence `json:"evidence"`
	Error         string   `json:"error,omitempty"`
}

type Server struct {
	Manager      *Manager
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (s *Server) Serve(ctx context.Context, listener *net.UnixListener) error {
	if s.Manager == nil || listener == nil {
		return errors.New("sandbox server: manager and Unix listener are required")
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("sandbox server: accept: %w", err)
		}
		s.handle(ctx, connection)
	}
}

func (s *Server) handle(ctx context.Context, connection *net.UnixConn) {
	defer connection.Close()
	readTimeout := s.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 5 * time.Second
	}
	writeTimeout := s.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 5 * time.Second
	}
	_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
	raw, err := readFrame(connection)
	if err != nil {
		return
	}
	request, err := decodeRunRequest(raw)
	if err != nil {
		_ = connection.SetWriteDeadline(time.Now().Add(writeTimeout))
		_ = writeFrame(connection, RunResponse{SchemaVersion: ProtocolSchemaVersion, Error: err.Error()})
		return
	}

	_ = connection.SetReadDeadline(time.Time{})
	runCtx, cancel := context.WithCancel(ctx)
	disconnected := make(chan bool, 1)
	go func() {
		var extra [1]byte
		_, readErr := connection.Read(extra[:])
		disconnected <- readErr == nil || !isTimeout(readErr)
	}()
	done := make(chan struct{})
	var evidence Evidence
	var runErr error
	go func() {
		evidence, runErr = s.Manager.Run(runCtx, request.Specification)
		close(done)
	}()
	clientGone := false
	select {
	case clientGone = <-disconnected:
		if clientGone {
			cancel()
		}
		<-done
	case <-done:
		_ = connection.SetReadDeadline(time.Now())
		clientGone = <-disconnected
	}
	cancel()
	if clientGone {
		return
	}
	response := RunResponse{SchemaVersion: ProtocolSchemaVersion, Evidence: evidence}
	if runErr != nil {
		response.Error = runErr.Error()
	}
	_ = connection.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = writeFrame(connection, response)
}

func PreparePrivateDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("sandboxd private directory must be an absolute clean path")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 || int(stat.Uid) != os.Getuid() {
		return errors.New("sandboxd private directory must be an operator-owned mode 0700 directory")
	}
	return nil
}

func ListenUnix(path string) (*net.UnixListener, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("sandboxd socket path must be absolute and clean")
	}
	if err := PreparePrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 || int(stat.Uid) != os.Getuid() {
			return nil, errors.New("sandboxd refuses unsafe existing socket path")
		}
		connection, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return nil, errors.New("sandboxd is already listening")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 || int(stat.Uid) != os.Getuid() {
		_ = listener.Close()
		return nil, errors.New("sandboxd could not establish an operator-owned mode 0600 socket")
	}
	return listener, nil
}

func decodeRunRequest(raw []byte) (RunRequest, error) {
	if err := rejectDuplicateFields(raw); err != nil {
		return RunRequest{}, fmt.Errorf("sandboxd protocol: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request RunRequest
	if err := decoder.Decode(&request); err != nil {
		return RunRequest{}, fmt.Errorf("sandboxd protocol: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return RunRequest{}, errors.New("sandboxd protocol: multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return RunRequest{}, fmt.Errorf("sandboxd protocol: %w", err)
	}
	if request.SchemaVersion != ProtocolSchemaVersion {
		return RunRequest{}, fmt.Errorf("sandboxd protocol: unsupported schema_version %q", request.SchemaVersion)
	}
	return request, nil
}

func readFrame(reader io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxProtocolFrame {
		return nil, errors.New("sandboxd protocol frame is empty or oversized")
	}
	raw := make([]byte, size)
	_, err := io.ReadFull(reader, raw)
	return raw, err
}

func writeFrame(writer io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 || len(raw) > maxProtocolFrame {
		return errors.New("sandboxd protocol response is empty or oversized")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(raw)))
	if _, err := writer.Write(header[:]); err != nil {
		return err
	}
	_, err = writer.Write(raw)
	return err
}

func isTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
