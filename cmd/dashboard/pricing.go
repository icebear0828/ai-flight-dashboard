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

var pricingTableURLs = []string{
	"https://raw.githubusercontent.com/icebear0828/token-ray/main/cmd/dashboard/pricing_table.json",
}

func fetchDynamicPricingFromURLs(urls []string, timeout time.Duration) ([]byte, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no pricing URLs configured")
	}
	var lastErr error
	for _, url := range urls {
		data, err := fetchDynamicPricing(url, timeout)
		if err == nil {
			return data, nil
		}
		lastErr = fmt.Errorf("%s: %w", url, err)
	}
	return nil, lastErr
}

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
