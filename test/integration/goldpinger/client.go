// +build integration

package goldpinger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	Host string
}

func (c *Client) CheckAll(ctx context.Context) (CheckAllJSON, error) {
	endpoint := fmt.Sprintf("%s/check_all", c.Host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CheckAllJSON{}, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return CheckAllJSON{}, err
	}
	defer res.Body.Close()

	var jsonResp CheckAllJSON
	if err := json.NewDecoder(res.Body).Decode(&jsonResp); err != nil {
		return CheckAllJSON{}, err
	}

	return jsonResp, nil
}
