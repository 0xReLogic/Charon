package main

import (
	"flag"
	testutil "github.com/0xReLogic/Charon/test"
	"log"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "proxy address")
	flag.Parse()
	if err := testutil.RunInteractiveProxyClient(*addr); err != nil {
		log.Fatal(err)
	}
}
