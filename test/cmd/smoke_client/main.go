package main

import (
	"flag"
	"fmt"
	"log"
	"time"
	
	"github.com/0xReLogic/Charon/testutil"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "proxy address")
	msg := flag.String("msg", "hello-through-proxy\n", "message to send")
	to := flag.Duration("timeout", 2*time.Second, "dial timeout")
	flag.Parse()
	if err := testutil.RunSmokeClient(*addr, *msg, *to); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Smoke test OK")
}
