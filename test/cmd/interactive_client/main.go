package main

import (
	"flag"
	"log"
	
	"github.com/0xReLogic/Charon/testutil"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "proxy address")
	flag.Parse()
	if err := testutil.RunInteractiveProxyClient(*addr); err != nil {
		log.Fatal(err)
	}
}
