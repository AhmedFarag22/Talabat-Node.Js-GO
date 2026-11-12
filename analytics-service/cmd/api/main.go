package main

import (
	"fmt"
	"os"

	"github.com/quickbite/analytics-service/lib/boot"
)

func main() {
	if err := boot.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "boot failed:", err)
		os.Exit(1)
	}
}
