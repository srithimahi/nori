package transfer

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

const MaxFileSize int64 = 10 << 30
const MaxNameLength = 255
const ChunkSize = 256 * 1024

func SendFile(conn net.Conn, filePath string) error {
	fmt.Printf("DEBUG: Opening file %q\n", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return errors.New("only regular files are supported")
	}

	if info.Size() > MaxFileSize {
		return errors.New("files exceeds the 10 GiB limit")
	}

	name := filepath.Base(filePath)

	if len(name) == 0 || len(name) > MaxNameLength {
		return errors.New("invalid filename length")
	}

	if err := binary.Write(conn, binary.BigEndian, uint16(len(name))); err != nil {
		return err
	}

	if _, err := io.WriteString(conn, name); err != nil {
		return err
	}

	if err := binary.Write(conn, binary.BigEndian, uint64(info.Size())); err != nil {
		return err
	}

	fmt.Printf("Sending %s (%d bytes)\n", name, info.Size())

	buffer := make([]byte, ChunkSize)
	hasher := sha256.New()

	var totalSent int64

	for totalSent < info.Size() {
		remaining := info.Size() - totalSent
		nextSize := int64(ChunkSize)

		if remaining < nextSize {
			nextSize = remaining
		}

		n, err := io.ReadFull(file, buffer[:int(nextSize)])
		if err != nil {
			return fmt.Errorf("reading chunk: %w", err)
		}

		if _, err := hasher.Write(buffer[:n]); err != nil {
			return err
		}

		written := 0
		for written < n {
			count, err := conn.Write(buffer[written:n])
			if err != nil {
				return fmt.Errorf("sending chunk: %w", err)
			}

			if count == 0 {
				return io.ErrShortWrite
			}

			written += count
		}

		totalSent += int64(n)

		fmt.Printf("\rSending: %.1f%%",
			float64(totalSent)/float64(info.Size())*100)
	}

	fmt.Println()
	fmt.Printf("SHA-256: %x\n", hasher.Sum(nil))
	fmt.Printf("Sent %d bytes\n", totalSent)

	return nil
}

func ReceiveFile(conn net.Conn, downloadDir string) error {
	var nameLength uint16

	if err := binary.Read(conn, binary.BigEndian, &nameLength); err != nil {
		return err
	}

	if nameLength == 0 || nameLength > MaxNameLength {
		return errors.New("invalid filename length")
	}

	nameBytes := make([]byte, nameLength)

	if _, err := io.ReadFull(conn, nameBytes); err != nil {
		return err
	}

	name := string(nameBytes)

	if name == "." || name == ".." || filepath.Base(name) != name || filepath.IsAbs(name) {
		return errors.New("unsafe filename")
	}

	var fileSize uint64

	if err := binary.Read(conn, binary.BigEndian, &fileSize); err != nil {
		return err
	}

	if fileSize > uint64(MaxFileSize) {
		return errors.New("incoming file exceeds size limit")
	}

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(downloadDir, ".nori-*.part")
	if err != nil {
		return err
	}

	tempPath := temp.Name()
	defer os.Remove(tempPath)

	fmt.Printf("Receiving %s (%d bytes)\n", name, fileSize)

	buffer := make([]byte, ChunkSize)
	hasher := sha256.New()

	var totalReceived int64

	for totalReceived < int64(fileSize) {
		remaining := int64(fileSize) - totalReceived
		nextSize := int64(ChunkSize)

		if remaining < nextSize {
			nextSize = remaining
		}

		n, err := io.ReadFull(conn, buffer[:int(nextSize)])
		if err != nil {
			temp.Close()
			return fmt.Errorf("receiving chuck: %w", err)
		}

		written, err := temp.Write(buffer[:n])
		if err != nil {
			temp.Close()
			return fmt.Errorf("writing chunk: %w", err)
		}
		if written != n {
			temp.Close()
			return io.ErrShortWrite
		}

		if _, err := hasher.Write(buffer[:n]); err != nil {
			temp.Close()
			return err
		}

		totalReceived += int64(n)

		fmt.Printf("\rReceiving: %1ff%%",
			float64(totalReceived)/float64(fileSize)*100)
	}

	fmt.Println()
	fmt.Printf("SHA-256: %x\n", hasher.Sum(nil))

	received := totalReceived

	if err := temp.Close(); err != nil {
		return err
	}

	base := filepath.Base(name)
	destination := filepath.Join(downloadDir, base)

	for i := 1; ; i++ {
		_, err := os.Stat(destination)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return err
		}

		ext := filepath.Ext(base)
		stem := base[:len(base)-len(ext)]
		destination = filepath.Join(downloadDir,
			fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}

	if err := os.Rename(tempPath, destination); err != nil {
		return err
	}

	fmt.Printf("Received %d bytes \n", received)
	fmt.Println("Saved to:", destination)

	return nil
}
