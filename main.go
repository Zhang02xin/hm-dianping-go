package main

import (
	"log"

	"github.com/Zhang02xin/hm-dianping-go/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
