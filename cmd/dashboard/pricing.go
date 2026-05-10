package main

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"time"
)

//go:embed pricing_table.json
var embeddedPricing []byte

func fetchDynamicPricing(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}
