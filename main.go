package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	files := NewFiles()

	_mux := mux.NewRouter()
	_mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status": "ok"}`))
	})
	_mux.HandleFunc("/download/{guid}", files.Download)
	_mux.HandleFunc("/upload/{filename}", files.Upload)

	fmt.Println("Floppa booting")
	if err := http.ListenAndServe(":8000", _mux); err != nil {
		log.Fatal(err)
	}
}
