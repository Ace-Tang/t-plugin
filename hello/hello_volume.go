package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/Sirupsen/logrus"
)

var (
	driverName = "hello"
	runDir     = fmt.Sprintf("/var/run/docker/%s", driverName)
	sockPath   = fmt.Sprintf("%s/%s.sock", runDir, driverName)
	stLock     sync.RWMutex
)

func main() {
	os.Mkdir(runDir, 0700)
	logrus.Errorf("exit with error %s", startPlugin())
}

func startPlugin() error {
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		logrus.Errorf("docker volume plugin listen error %v", err)
		return err
	}

	//TODO: set signal and catch signal

	m := http.NewServeMux()

	m.HandleFunc("/Plugin/Volume.Activate", makeHandler(activate))
	m.HandleFunc("/Plugin/Volume.Create", makeHandler(create))
	m.HandleFunc("/Plugin/Volume.Remove", makeHandler(remove))
	m.HandleFunc("/Plugin/Volume.Mount", makeHandler(mount))
	m.HandleFunc("/Plugin/Volume.Path", makeHandler(path))
	m.HandleFunc("/Plugin/Volume.Unmount", makeHandler(unmount))
	m.HandleFunc("/Plugin/Volume.Get", makeHandler(get))
	m.HandleFunc("/Plugin/Volume.List", makeHandler(list))
	m.HandleFunc("/Plugin/Volume.Capabilities", makeHandler(capabilities))

	return http.Serve(l, m)

}

func makeHandler(h func(r *http.Request, w http.ResponseWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(r, w)
	}
}

func activate(r *http.Request, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	data := map[string][]string{
		"Implements": []string{"VolumeDriver"},
	}
	json.NewEncoder(w).Encode(data)
}

func create(r *http.Request, w http.ResponseWriter) {

}

func remove(r *http.Request, w http.ResponseWriter) {

}

func mount(r *http.Request, w http.ResponseWriter) {

}

func path(r *http.Request, w http.ResponseWriter) {

}

func unmount(r *http.Request, w http.ResponseWriter) {

}

func get(r *http.Request, w http.ResponseWriter) {

}

func list(r *http.Request, w http.ResponseWriter) {

}

func capabilities(r *http.Request, w http.ResponseWriter) {

}
