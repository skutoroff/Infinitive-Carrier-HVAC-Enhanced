package main
import (
    "log"
    "net/http"
)
func main() {
    // Simple static webserver
    fs := http.FileServer(http.Dir("/var/lib/infinitive"))
    http.Handle("/infinitive/", http.StripPrefix("/infinitive/", fs))

    err:= http.ListenAndServe(":8081", nil)
    if err != nil {
        log.Fatal("staticServer ListenAndServe fail", err)
    }
}
