package toolbox

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	HTTP     = "http"
	HTTPPort = 8080
	TCP      = "tcp"
	TCPPort  = 8085
	UDP      = "udp"
	UDPPort  = 8086

	buffersize = 1024
)

func Main() {
	tcpPort, err := strconv.Atoi(os.Getenv("TCP_PORT"))
	if err != nil {
		tcpPort = TCPPort
		fmt.Printf("TCP_PORT not set, defaulting to port %d\n", TCPPort)
	}

	udpPort, err := strconv.Atoi(os.Getenv("UDP_PORT"))
	if err != nil {
		udpPort = UDPPort
		fmt.Printf("UDP_PORT not set, defaulting to port %d\n", UDPPort)
	}

	httpPort, err := strconv.Atoi(os.Getenv("HTTP_PORT"))
	if err != nil {
		httpPort = HTTPPort
		fmt.Printf("HTTP_PORT not set, defaulting to port %d\n", HTTPPort)
	}

	go ListenOnUDP(udpPort)
	go ListenOnTCP(tcpPort)
	ListenHTTP(httpPort)
}

func ListenHTTP(port int) {
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Printf("[HTTP] Received Connection from %v\n", r.RemoteAddr)
		_, err := rw.Write(getResponse(r.RemoteAddr, "http"))
		if err != nil {
			fmt.Println(err)
		}
	})

	p := strconv.Itoa(port)
	fmt.Printf("[HTTP] Listening on %+v\n", p)

	if err := http.ListenAndServe(":"+p, nil); err != nil {
		panic(err)
	}
}

func ListenOnTCP(port int) {
	listener, err := net.ListenTCP(TCP, &net.TCPAddr{Port: port})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer listener.Close()

	fmt.Printf("[TCP] Listening on %+v\n", listener.Addr().String())
	rand.Seed(time.Now().Unix())

	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		go handleConnection(connection)
	}
}

func handleConnection(connection net.Conn) {
	addressString := fmt.Sprintf("%+v", connection.RemoteAddr())
	fmt.Printf("[TCP] Received Connection from %s\n", addressString)
	_, err := connection.Write(getResponse(addressString, TCP))
	if err != nil {
		fmt.Println(err)
	}

	err = connection.Close()
	if err != nil {
		fmt.Println(err)
	}
}

func getResponse(addressString, protocol string) []byte {
	hostname, _ := os.Hostname()
	interfaces, _ := net.Interfaces()
	var base string
	for _, iface := range interfaces {
		base += fmt.Sprintf("\t%+v\n", iface.Name)
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			base += fmt.Sprintf("\t\t%+v\n", addr)
		}
	}

	return []byte(fmt.Sprintf("Connected To: %s via %s\nConnected From: %v\nRemote Interfaces:\n%v", hostname, protocol, addressString, base))
}

func ListenOnUDP(port int) {
	connection, err := net.ListenUDP(UDP, &net.UDPAddr{Port: port})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("[UDP] Listening on %+v\n", connection.LocalAddr().String())

	defer connection.Close()
	buffer := make([]byte, buffersize)
	rand.Seed(time.Now().Unix())

	for {
		n, addr, err := connection.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
		}
		payload := strings.TrimSpace(string(buffer[0 : n-1]))

		if payload == "STOP" {
			fmt.Println("Exiting UDP server")
			return
		}

		addressString := fmt.Sprintf("%+v", addr)
		fmt.Printf("[UDP] Received Connection from %s\n", addressString)
		_, err = connection.WriteToUDP(getResponse(addressString, UDP), addr)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}
