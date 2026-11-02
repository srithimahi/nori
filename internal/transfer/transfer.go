package transfer

import(
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

	sent, err := io.CopyN(conn, file, info.Size())
	if err != nil {
		return err
	}

	fmt.Printf("Send %d bytes\n", sent)
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

	if _,err := io.ReadFull(conn, nameBytes); err != nil {
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

	received, err := io.CopyN(temp, conn, int64(fileSize))
	if err != nil {
		temp.Close()
		return fmt.Errorf("incomplete transfer: %w", err)
	}

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