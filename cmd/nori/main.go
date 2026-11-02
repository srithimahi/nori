package main

import(
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/srithimahi/nori/internal/transfer"
)

func main() {
	mode := flag.String("mode", "", "listen, connect, send, or receive")
	address := flag.String("address", "127.0.0.1:8000", "TCP address")
	filePath := flag.String("file", "", "File to send")
	fmt.Printf("RAW ARGS: %#v\n", os.Args)
	flag.Parse()

	fmt.Println("=============")
	fmt.Println("     nori")
	fmt.Println("=============")

	switch *mode {
	case "listen":
		startServer(*address)
	case "connect":
		startClient(*address)
	case "send":
		fmt.Printf("MAIN DEBUG: filepath = %q\n", *filePath)
		if *filePath == "" {
			fmt.Println("Please provide -file")
			return
		}
		sendFile(*address, *filePath)

	case "receive":
		receiveFiles(*address)
	default: 
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/nori -mode=listen")
		fmt.Println("  go run ./cmd/nori -mode=connect")
		fmt.Println("  go run ./cmd/nori -mode=receive")
		fmt.Println("  go run ./cmd/nori -mode=send -file=example.txt")
	} 
}

func startServer(address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Listener error:", err)
		return
	}

	defer listener.Close()

	fmt.Println("Listening on", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Connection error:", err)
			continue
		}

		fmt.Println("New peer:", conn.RemoteAddr())
		go handleConnection(conn)
	}
}

func startClient(address string) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}

	fmt.Println("Connected to", conn.RemoteAddr())
	handleConnection(conn)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Println("Type message or /quit to disconnect")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		reader := bufio.NewReader(conn)

		for {
			message, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Println("Read error:", err)
				}

				fmt.Println("\nPeer disconnected")
				return
			}

			fmt.Printf("\nPeer: %s", message)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		message := scanner.Text()

		if strings.TrimSpace(message) == "/quit" {
			break
		}

		if _, err := fmt.Fprintln(conn, message); err != nil {
			fmt.Println("Send error:", err)
			break
		}
	}

	conn.Close()
	wg.Wait()
}

func receiveFiles(address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Receiver error:", err)
		return
	}
	defer listener.Close()

	fmt.Println("Ready to receive files on", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}

		go func(c net.Conn) {
			defer c.Close()

			fmt.Println("Incoming transfer from", c.RemoteAddr())

			if err := transfer.ReceiveFile(c, "downloads"); err != nil {
				fmt.Println("Transfer failed:", err)
			}
		}(conn)
	}
}

func sendFile(address, filePath string) {
	fmt.Println("Starting file transfer")

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("Connection error: ", err)
		return
	}
	defer conn.Close()

	if err := transfer.SendFile(conn, filePath); err != nil {
		fmt.Println("Transfer failed: ", err)
		return
	}

	fmt.Println("File byte sent successfully")
}