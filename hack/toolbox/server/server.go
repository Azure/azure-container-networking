package main

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
	httpport = 8080
	tcp      = "tcp"
	tcpport  = 8085
	udp      = "udp"
	udpport  = 8086

	buffersize = 1024
)

func main() {
	tcpPort, err := strconv.Atoi(os.Getenv("TCP_PORT"))
	if err != nil {
		tcpPort = tcpport
		fmt.Printf("TCP_PORT not set, defaulting to port %d\n", tcpport)
	}

	udpPort, err := strconv.Atoi(os.Getenv("UDP_PORT"))
	if err != nil {
		udpPort = udpport
		fmt.Printf("UDP_PORT not set, defaulting to port %d\n", udpport)
	}

	httpPort, err := strconv.Atoi(os.Getenv("HTTP_PORT"))
	if err != nil {
		httpPort = httpport
		fmt.Printf("HTTP_PORT not set, defaulting to port %d\n", httpport)
	}

	go listenOnUDP(udpPort)
	go listenOnTCP(tcpPort)
	listenHTTP(httpPort)
}

func listenHTTP(port int) {
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

func listenOnTCP(port int) {
	listener, err := net.ListenTCP(tcp, &net.TCPAddr{Port: port})
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
	_, err := connection.Write(getResponse(addressString, tcp))
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

func listenOnUDP(port int) {
	connection, err := net.ListenUDP(udp, &net.UDPAddr{Port: port})
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
		_, err = connection.WriteToUDP(getResponse(addressString, udp), addr)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}
