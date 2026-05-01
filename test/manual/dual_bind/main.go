// Manual test: verify multicast + broadcast can coexist on port 9101.
// Run with: go run ./test/manual/dual_bind/
package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	addr1, _ := net.ResolveUDPAddr("udp", "224.0.0.123:9101")
	conn1, err := net.ListenMulticastUDP("udp", nil, addr1)
	if err != nil {
		fmt.Println("Multicast listen error:", err)
	} else {
		fmt.Println("Multicast listening")
		defer conn1.Close()
	}

	addr2, _ := net.ResolveUDPAddr("udp", "0.0.0.0:9101")
	conn2, err := net.ListenUDP("udp", addr2)
	if err != nil {
		fmt.Println("Broadcast listen error:", err)
	} else {
		fmt.Println("Broadcast listening")
		defer conn2.Close()
	}

	time.Sleep(1 * time.Second)
}
