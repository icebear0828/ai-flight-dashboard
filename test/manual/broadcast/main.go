// Manual test: UDP broadcast send + receive on port 9101.
// Run with: go run ./test/manual/broadcast/
package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	go func() {
		addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:9101")
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			fmt.Println("Listen error:", err)
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err == nil {
				fmt.Printf("Received broadcast: %s\n", string(buf[:n]))
			}
		}
	}()

	time.Sleep(1 * time.Second)

	addr, _ := net.ResolveUDPAddr("udp", "255.255.255.255:9101")
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Println("Dial error:", err)
		return
	}
	defer conn.Close()
	_, err = conn.Write([]byte("ping"))
	if err != nil {
		fmt.Println("Write error:", err)
	}

	time.Sleep(1 * time.Second)
}
