package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", HelloHandler)
	http.HandleFunc("/env", EnvHandler)
	http.HandleFunc("/ls", LsHandler)
	http.HandleFunc("/exit", ExitHandler)

	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("an unexpected error occurred: %w", err))
		os.Exit(1)
	}
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hi, I'm not Dora!")
}

func EnvHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, strings.Join(os.Environ(), "\n"))
}

func LsHandler(w http.ResponseWriter, r *http.Request) {
	paths := r.URL.Query()["path"]
	if len(paths) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "one path value expected, found %d in %v", len(paths), paths)

		return
	}

	files, err := ioutil.ReadDir(paths[0])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not list %s: %s", paths[0], err.Error())

		return
	}

	for _, file := range files {
		fmt.Fprintln(w, file.Name())
	}
}

func ExitHandler(w http.ResponseWriter, r *http.Request) {
	exitCodes, ok := r.URL.Query()["exitCode"]
	if !ok {
		os.Exit(0)
	}

	if len(exitCodes) != 1 {
		fmt.Fprintf(w, "one exit code value expected, found %d in %v", len(exitCodes), exitCodes)

		return
	}

	exitCode, err := strconv.Atoi(exitCodes[0])
	if err != nil {
		fmt.Fprintf(w, "invalid exit code value: %s", exitCodes)

		return
	}

	os.Exit(exitCode)
}
