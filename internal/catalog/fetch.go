package catalog

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 60 * time.Second}

// Fetch downloads and decodes the catalog. It is the only function in this
// program that talks to the network.
//
// The whole catalog is taken in one request — about 2.8 MB — because that is
// cheaper than asking for each product separately and removes any need for a
// prebuilt database shipped with the binary.
func Fetch() (*Catalog, error) {
	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "eolwhen (https://github.com/iwamot/eolwhen)")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w; check the network, then retry", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w; check the network, then retry", URL, err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%s: HTTP %d; endoflife.date is failing, retry later", URL, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected HTTP %d; retry once, and if it persists the API has moved", URL, resp.StatusCode)
	}
	return Decode(body)
}
