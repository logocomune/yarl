package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/logocomune/yarl/v2/integration/httpratelimit"
)

func main() {
	conf := httpratelimit.NewConfigurationWithLru("rl", 100, 2, 5*time.Second)
	conf.UseIP = true

	log.Println("Server start")
	http.Handle("/ping", httpratelimit.New(conf, pingHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong - "+time.Now().Format(time.RFC3339Nano))
}
