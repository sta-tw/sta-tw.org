package storage

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"testing"
)

type fakeScanner struct {
	path string
	err  error
}

func (f *fakeScanner) Scan(_ context.Context, path string) error {
	f.path = path
	return f.err
}

func TestScanStagedUploadDelegatesToScanner(t *testing.T) {
	scanner := &fakeScanner{err: errors.New("scan failed")}
	err := ScanStagedUpload(context.Background(), scanner, StagedUpload{Path: "/tmp/sta-upload"})
	if !errors.Is(err, scanner.err) || scanner.path != "/tmp/sta-upload" {
		t.Fatalf("ScanStagedUpload() = %v, scanner path = %q", err, scanner.path)
	}
}

func TestNewClamAVScannerAcceptsTCPAndUnixAddresses(t *testing.T) {
	for _, value := range []string{"127.0.0.1:3310", "tcp://clamav:3310", "unix:///var/run/clamav/clamd.ctl"} {
		if _, err := NewClamAVScanner(value); err != nil {
			t.Fatalf("NewClamAVScanner(%q) error = %v", value, err)
		}
	}
	if _, err := NewClamAVScanner("clamav"); err == nil {
		t.Fatal("NewClamAVScanner(invalid address) error = nil")
	}
}

func TestClamAVScannerStreamsFileAndAcceptsOK(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local socket is unavailable in this sandbox: %v", err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		command, readErr := reader.ReadBytes(0)
		if readErr != nil || string(command) != "zINSTREAM\x00" {
			serverErr <- errors.New("invalid ClamAV command")
			return
		}
		var length [4]byte
		for {
			if _, readErr := io.ReadFull(reader, length[:]); readErr != nil {
				serverErr <- readErr
				return
			}
			size := binary.BigEndian.Uint32(length[:])
			if size == 0 {
				break
			}
			if _, readErr := io.CopyN(io.Discard, reader, int64(size)); readErr != nil {
				serverErr <- readErr
				return
			}
		}
		// Real clamd terminates a zINSTREAM response with NUL, not a
		// newline — this test previously sent '\n' here, which let
		// Scan()'s ReadString('\n') bug (it never fixed the same
		// null-terminator detail Ping() already accounted for) pass
		// unnoticed until a real upload actually hung against real clamd.
		_, writeErr := connection.Write([]byte("stream: OK\x00"))
		serverErr <- writeErr
	}()

	temporary, err := os.CreateTemp("", "sta-clamav-test-*")
	if err != nil {
		t.Fatalf("os.CreateTemp() error = %v", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err := temporary.WriteString("synthetic upload"); err != nil {
		temporary.Close()
		t.Fatalf("write test file: %v", err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatalf("close test file: %v", err)
	}
	scanner, err := NewClamAVScanner(listener.Addr().String())
	if err != nil {
		t.Fatalf("NewClamAVScanner() error = %v", err)
	}
	if err := scanner.Scan(context.Background(), path); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("ClamAV test server error = %v", err)
	}
}

func TestClamAVScannerDetectsMalwareWithNULTerminatedResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local socket is unavailable in this sandbox: %v", err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		if _, readErr := reader.ReadBytes(0); readErr != nil {
			serverErr <- readErr
			return
		}
		var length [4]byte
		for {
			if _, readErr := io.ReadFull(reader, length[:]); readErr != nil {
				serverErr <- readErr
				return
			}
			if binary.BigEndian.Uint32(length[:]) == 0 {
				break
			}
			if _, readErr := io.CopyN(io.Discard, reader, int64(binary.BigEndian.Uint32(length[:]))); readErr != nil {
				serverErr <- readErr
				return
			}
		}
		_, writeErr := connection.Write([]byte("stream: Eicar-Test-Signature FOUND\x00"))
		serverErr <- writeErr
	}()

	temporary, err := os.CreateTemp("", "sta-clamav-test-*")
	if err != nil {
		t.Fatalf("os.CreateTemp() error = %v", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err := temporary.WriteString("synthetic malware payload"); err != nil {
		temporary.Close()
		t.Fatalf("write test file: %v", err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatalf("close test file: %v", err)
	}
	scanner, err := NewClamAVScanner(listener.Addr().String())
	if err != nil {
		t.Fatalf("NewClamAVScanner() error = %v", err)
	}
	if err := scanner.Scan(context.Background(), path); !errors.Is(err, ErrMalwareDetected) {
		t.Fatalf("Scan() error = %v, want ErrMalwareDetected", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("ClamAV test server error = %v", err)
	}
}
