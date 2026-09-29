// Command replay runs a recorded Azure scenario without a subscription.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rmkr-dev/sentinelmesh/internal/replay"
)

func main() {
	inc, err := replay.StorageThrottling(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(inc); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
