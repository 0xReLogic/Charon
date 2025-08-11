package main

import (
	"flag"
	"log"
	testutil "github.com/0xReLogic/Charon/test"
)

func main() {
	port := flag.String("port", "9091", "Port to listen on")
	flag.Parse()
	if err := testutil.RunEchoServer(*port); err != nil {
		log.Fatal(err)
	}
}
