package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/logocomune/yarl/integration/httpratelimit"
)

func main() {
	conf := httpratelimit.NewConfigurationWithRadix("test_prefix", 5, "127.0.0.1", "16379", 0, 2, 5*time.Second)
	conf.UseIP = true

	log.Println("Server start")
	http.Handle("/ping", httpratelimit.New(conf, pingHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong - "+time.Now().Format(time.RFC3339Nano))
}
