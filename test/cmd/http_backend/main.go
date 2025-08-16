package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":9091", "HTTP backend listen address")
	flag.Parse()

	// Failing endpoint to help test circuit breaker: always returns 500
	http.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "fail\n")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "method=%s path=%s body=%s\n", r.Method, r.URL.Path, string(body))
	})

	log.Printf("HTTP backend listening on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}
