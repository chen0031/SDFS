package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"simpledfs/utils"
	"time"
)

const (
	BufferSize = 4096
)

// Usage of correct client command
func usage() {
	fmt.Println("Usage of ./client")
	fmt.Println("   -master=[master IP:Port] put [localfilename] [sdfsfilename]")
	fmt.Println("   -master=[master IP:Port] get [sdfsfilename] [localfilename]")
	fmt.Println("   -master=[master IP:Port] delete [sdfsfilename]")
	fmt.Println("   -master=[master IP:Port] ls [sdfsfilename]")
	fmt.Println("   -master=[master IP:Port] store")
	fmt.Println("   -master=[master IP:Port] get-versions [sdfsfilename] [num-versions] [localfilename]")
}

func contactNode(addr string) (net.Conn, error) {
	// Time out needed in order to deal with server failure.
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	printError(err)
	return conn, err
}

func filePut(pr utils.PutResponse, localfile string) {
	port := utils.StringPort(pr.NexthopPort)
	ip := utils.StringIP(pr.NexthopIP)

	conn, err := net.Dial("tcp", ip+":"+port)
	if err != nil {
		utils.PrintError(err)
		return
	}
	defer conn.Close()

	file, err := os.Open(localfile)
	utils.PrintError(err)

	wr := utils.WriteRequest{MsgType: utils.WriteRequestMsg}
	wr.FilenameHash = pr.FilenameHash
	wr.Filesize = pr.Filesize
	wr.Timestamp = pr.Timestamp
	wr.DataNodeList = pr.DataNodeList
	wr.DataNodeList[0] = 0

	bin := utils.Serialize(wr)
	_, err = conn.Write(bin)
	utils.PrintError(err)

	buf := make([]byte, BufferSize)

	n, err := conn.Read(buf)
	for string(buf[:n]) != "OK" {
	}
	fmt.Println(string(buf[:n]))

	buf = make([]byte, BufferSize)
	for {
		n, err := file.Read(buf)
		conn.Write(buf[:n])
		if err == io.EOF {
			fmt.Println("Send file finish")
			break
		}
	}
}

func fileGet(gr utils.GetResponse, localfile string) {
	var conn net.Conn
	hasConn := false
	for index, ip := range gr.DataNodeIPList {
		conn, err := net.Dial("tcp", utils.StringIP(ip)+":"+fmt.Sprintf("%d", gr.DataNodePortList[index]))
		utils.PrintError(err)
		if err == nil {
			hasConn = true
			break
		}
		defer conn.Close()
	}
	if hasConn == false {
		fmt.Println("Cannot dial all datanodes")
		return
	}

	file, err := os.Open(localfile)
	utils.PrintError(err)

	rr := utils.ReadRequest{MsgType: utils.ReadRequestMsg}
	rr.FilenameHash = gr.FilenameHash

	bin := utils.Serialize(rr)
	_, err = conn.Write(bin)
	utils.PrintError(err)

	conn.Write([]byte("OK"))

	buf := make([]byte, BufferSize)
	var receivedBytes uint64

	for {
		n, err := conn.Read(buf)
		file.Write(buf[:n])
		receivedBytes += uint64(n)
		if err == io.EOF {
			fmt.Printf("Receive file with %d bytes\n", receivedBytes)
			break
		}

	}

	if gr.Filesize != receivedBytes {
		fmt.Println("Unmatched two files")
	}
}

// Put command execution for simpleDFS client
func putCommand(masterConn net.Conn, sdfsfile string, filesize uint64, localfile string) {
	prPacket := utils.PutRequest{MsgType: utils.PutRequestMsg, Filesize: filesize}
	copy(prPacket.Filename[:], sdfsfile)

	bin := utils.Serialize(prPacket)
	_, err := masterConn.Write(bin)
	printErrorExit(err)

	buf := make([]byte, BufferSize)
	n, err := masterConn.Read(buf)
	printErrorExit(err)

	response := utils.PutResponse{}
	utils.Deserialize(buf[:n], &response)
	if response.MsgType != utils.PutResponseMsg {
		fmt.Println("Unexpected message from MasterNode")
		return
	}
	fmt.Printf("%s %d %v\n", utils.Hash2Text(response.FilenameHash[:]), response.Timestamp, response.DataNodeList)

	filePut(response, localfile)
}

// Get command execution for simpleDFS client
func getCommand(masterConn net.Conn, sdfsfile string, localfile string) {
	grPacket := utils.GetRequest{MsgType: utils.GetRequestMsg}
	copy(grPacket.Filename[:], sdfsfile)

	bin := utils.Serialize(grPacket)
	_, err := masterConn.Write(bin)
	printErrorExit(err)

	buf := make([]byte, BufferSize)
	n, err := masterConn.Read(buf)
	printErrorExit(err)

	response := utils.GetResponse{}
	utils.Deserialize(buf[:n], &response)
	if response.MsgType != utils.GetResponseMsg {
		fmt.Println("Unexpected message from MasterNode")
		return
	}

	fileGet(response, localfile)
}

func main() {
	// If no command line arguments, return
	if len(os.Args) <= 1 {
		usage()
		return
	}
	ipPtr := flag.String("master", "xx.xx.xx.xx:port", "Master's IP:Port address")
	flag.Parse()
	masterAddr := *ipPtr
	fmt.Println("Master IP:Port address ", masterAddr)
	masterConn, err := contactNode(masterAddr)
	if err != nil {
		return
	}

	args := flag.Args()
	command := args[0]
	switch command {
	case "put":
		if len(args) != 3 {
			fmt.Println("Invalid put usage")
			usage()
		}
		fmt.Println(args[1:])
		localfile := args[1]
		sdfsfile := args[2]
		filenode, err := os.Stat(localfile)
		if err != nil {
			printError(err)
			return
		}
		fmt.Println(filenode.Size(), sdfsfile)
		putCommand(masterConn, sdfsfile, uint64(filenode.Size()), localfile)
	case "get":
		if len(args) != 3 {
			fmt.Println("Invalid get usage")
			usage()
		}
		fmt.Println(args[1:])
		sdfsfile := args[1]
		localfile := args[2]
		getCommand(masterConn, sdfsfile, localfile)
	case "delete":
		if len(args) != 2 {
			fmt.Println("Invalid delete usage")
			usage()
		}
		fmt.Println(args[1:])
	case "ls":
		if len(args) != 2 {
			fmt.Println("Invalid ls usage")
			usage()
		}
		fmt.Println(args[1:])
	case "store":
		if len(args) != 1 {
			fmt.Println("Invalid store usage")
			usage()
		}
		fmt.Println(args[1:])
	case "get-versions":
		if len(args) != 4 {
			fmt.Println("Invalid get-versions usage")
			usage()
		}
		fmt.Println(args[1:])
	default:
		usage()
	}

}

// Helper function to print the err in process
func printError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERROR]", err.Error())
		fmt.Println(" ")
	}
}

func printErrorExit(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERROR]\n", err.Error())
		fmt.Println(" ")
		os.Exit(1)
	}
}
