package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const ServiceBindingRootEnv = "SERVICE_BINDING_ROOT"

func main() {
	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}

	if arg != "" && arg != "web" {
		for range time.Tick(time.Second * 1) {
			fmt.Printf("Hello from %q\n", arg)
		}

		return
	}

	http.HandleFunc("/", helloWorldHandler(arg))
	http.HandleFunc("/env.json", envJsonHandler)
	http.HandleFunc("/servicebindingroot", serviceBindingRootHandler)
	http.HandleFunc("/servicebindings", serviceBindingsHandler)
	http.HandleFunc("/exit", exitHandler)
	http.HandleFunc("/log", logHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Listening on port %s\n", port)
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func exitHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Fprintf(w, "Failed to parse form: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	code := r.Form.Get("code")
	if code == "" {
		code = "0"
	}

	exitCode, err := strconv.Atoi(code)
	if err != nil {
		fmt.Fprintf(w, "Failed to parse exit code: %s: %v", code, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	os.Exit(exitCode)
}

func logHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Fprintf(w, "Failed to parse form: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println(r.Form.Get("message"))
}

func helloWorldHandler(arg string) func(w http.ResponseWriter, _ *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		if arg == "" {
			fmt.Fprintln(w, "Hi, I'm Dorifi!")
		} else {
			fmt.Fprintf(w, "Hi, I'm Dorifi (%s)!\n", arg)
		}
	}
}

func envJsonHandler(w http.ResponseWriter, _ *http.Request) {
	envJson := map[string]string{}
	env := os.Environ()
	for _, kvPair := range env {
		elements := strings.Split(kvPair, "=")
		envJson[elements[0]] = elements[1]
	}

	if err := json.NewEncoder(w).Encode(envJson); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not print environment: %s", err.Error())
		return
	}
}

func serviceBindingRootHandler(w http.ResponseWriter, _ *http.Request) {
	serviceBindingRoot := os.Getenv(ServiceBindingRootEnv)
	if serviceBindingRoot == "" {
		fmt.Fprintln(w, "$SERVICE_BINDING_ROOT is empty")
		return
	}

	fmt.Fprintln(w, serviceBindingRoot)
	dirs, err := os.ReadDir(serviceBindingRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, dir := range dirs {
		dirPath := filepath.Join(serviceBindingRoot, dir.Name())
		fmt.Fprintln(w, dirPath)

		files, err := os.ReadDir(dirPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, file := range files {
			filePath := filepath.Join(dirPath, file.Name())
			fmt.Fprintln(w, filePath)
		}
	}
}

func serviceBindingsHandler(w http.ResponseWriter, _ *http.Request) {
	serviceBindingRoot := os.Getenv(ServiceBindingRootEnv)
	bindings := make(map[string]interface{})
	if serviceBindingRoot != "" {
		var err error
		bindings, err = getBindings(serviceBindingRoot, bindings)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	jsonBytes, err := json.Marshal(bindings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func getBindings(dir string, bindings map[string]interface{}) (map[string]interface{}, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() {
			subdir := filepath.Join(dir, file.Name())
			subfiles, err := os.ReadDir(subdir)
			if err != nil {
				return nil, err
			}
			secretData := make(map[string]string)
			for _, subfile := range subfiles {
				if !subfile.IsDir() && !strings.HasPrefix(subfile.Name(), ".") {

					// Keys in the mounted Secret are symbolic links. Get the target and process it
					target, err := os.Readlink(filepath.Join(subdir, subfile.Name()))
					if err != nil {
						return nil, err
					}

					targetContents, err := os.ReadFile(filepath.Join(subdir, target))
					if err != nil {
						return nil, err
					}

					secretData[subfile.Name()] = string(targetContents)
				}
			}
			if len(secretData) > 0 {
				bindings[file.Name()] = secretData
			}
		}
	}
	return bindings, nil
}
