package main

import (
	"fmt"
	"net"
)

func main() {
	_, ipnet, _ := net.ParseCIDR("192.168.10.5/24")
	fmt.Printf("IP: %v, Mask: %v, len(Mask): %d\n", ipnet.IP, ipnet.Mask, len(ipnet.Mask))
}
