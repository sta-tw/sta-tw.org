package storage

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

var (
	ErrMalwareDetected    = errors.New("malware detected in uploaded file")
	ErrScannerUnavailable = errors.New("file scanner is unavailable")
)

type Scanner interface {
	Scan(context.Context, string) error
}

type ClamAVScanner struct {
	network string
	address string
	timeout time.Duration
}

func NewClamAVScanner(rawAddress string) (*ClamAVScanner, error) {
	addressValue := strings.TrimSpace(rawAddress)
	if addressValue == "" || len(addressValue) > 512 || strings.ContainsAny(addressValue, "\x00\r\n") {
		return nil, errors.New("ClamAV address is invalid")
	}
	network := "tcp"
	address := addressValue
	switch {
	case strings.HasPrefix(addressValue, "tcp://"):
		address = strings.TrimPrefix(addressValue, "tcp://")
	case strings.HasPrefix(addressValue, "unix://"):
		network = "unix"
		address = strings.TrimPrefix(addressValue, "unix://")
	case strings.Contains(addressValue, "://"):
		return nil, errors.New("ClamAV address scheme is unsupported")
	}
	if address == "" || (network == "tcp" && !strings.Contains(address, ":")) {
		return nil, errors.New("ClamAV address is invalid")
	}
	return &ClamAVScanner{network: network, address: address, timeout: 30 * time.Second}, nil
}

func (s *ClamAVScanner) Scan(ctx context.Context, path string) error {
	if s == nil {
		return ErrScannerUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file for malware scan: %w", err)
	}
	defer file.Close()

	dialer := net.Dialer{Timeout: s.timeout}
	connection, err := dialer.DialContext(ctx, s.network, s.address)
	if err != nil {
		return fmt.Errorf("connect to ClamAV: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(s.timeout))
	}

	if err := writeAll(connection, []byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("start ClamAV stream: %w", err)
	}
	chunk := make([]byte, 32<<10)
	length := make([]byte, 4)
	for {
		readCount, readErr := file.Read(chunk)
		if readCount > 0 {
			binary.BigEndian.PutUint32(length, uint32(readCount))
			if err := writeAll(connection, length); err != nil {
				return fmt.Errorf("send ClamAV chunk: %w", err)
			}
			if err := writeAll(connection, chunk[:readCount]); err != nil {
				return fmt.Errorf("send ClamAV data: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read file for malware scan: %w", readErr)
		}
	}
	binary.BigEndian.PutUint32(length, 0)
	if err := writeAll(connection, length); err != nil {
		return fmt.Errorf("finish ClamAV stream: %w", err)
	}
	response, err := bufio.NewReader(io.LimitReader(connection, 4<<10)).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read ClamAV response: %w", err)
	}
	message := strings.TrimSpace(string(response))
	if strings.HasSuffix(message, "FOUND") || strings.Contains(message, "FOUND") {
		return ErrMalwareDetected
	}
	if !strings.HasSuffix(message, "OK") {
		return fmt.Errorf("%w: %s", ErrScannerUnavailable, message)
	}
	return nil
}

// Ping issues a clamd PING and expects PONG. Suitable for a readiness probe.
func (s *ClamAVScanner) Ping(ctx context.Context) error {
	if s == nil {
		return ErrScannerUnavailable
	}
	dialer := net.Dialer{Timeout: s.timeout}
	connection, err := dialer.DialContext(ctx, s.network, s.address)
	if err != nil {
		return fmt.Errorf("connect to ClamAV: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(s.timeout))
	}
	if err := writeAll(connection, []byte("zPING\x00")); err != nil {
		return fmt.Errorf("send ClamAV ping: %w", err)
	}
	response, err := bufio.NewReader(io.LimitReader(connection, 64)).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read ClamAV ping response: %w", err)
	}
	if strings.TrimSpace(response) != "PONG" {
		return fmt.Errorf("%w: unexpected ping response %q", ErrScannerUnavailable, strings.TrimSpace(response))
	}
	return nil
}

func ScanStagedUpload(ctx context.Context, scanner Scanner, upload StagedUpload) error {
	if scanner == nil {
		return nil
	}
	if upload.Path == "" {
		return ErrScannerUnavailable
	}
	err := scanner.Scan(ctx, upload.Path)
	if err == nil || errors.Is(err, ErrMalwareDetected) || errors.Is(err, ErrScannerUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrScannerUnavailable, err)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
