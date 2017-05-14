package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
)

var (
	driverName = "hello"
	runDir     = fmt.Sprintf("/run/docker/plugins/%s", driverName)
	sockPath   = fmt.Sprintf("%s/%s.sock", runDir, driverName)
	stLock     sync.Mutex
	store      = make(map[string]*vol)
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
	ShutDownTrap(func() {
		l.Close()
	})

	m := http.NewServeMux()

	m.HandleFunc("/Plugin.Activate", makeHandler(activate))
	m.HandleFunc("/VolumeDriver.Create", makeHandler(create))
	m.HandleFunc("/VolumeDriver.Remove", makeHandler(remove))
	m.HandleFunc("/VolumeDriver.Mount", makeHandler(mount))
	m.HandleFunc("/VolumeDriver.Path", makeHandler(path))
	m.HandleFunc("/VolumeDriver.Unmount", makeHandler(unmount))
	m.HandleFunc("/VolumeDriver.Get", makeHandler(get))
	m.HandleFunc("/VolumeDriver.List", makeHandler(list))
	m.HandleFunc("/VolumeDriver.Capabilities", makeHandler(capabilities))

	return http.Serve(l, m)

}

func makeHandler(h func(r *http.Request, w http.ResponseWriter) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(r, w); err != nil {
			logrus.Errorf("handle for methed %s return error %v", r.Method, err)
			WriteHttpError(w, err)
		}
	}
}

func activate(r *http.Request, w http.ResponseWriter) error {
	data := map[string][]string{
		"Implements": []string{"VolumeDriver"},
	}
	return WriteHttpJson(w, http.StatusOK, data)
}

func create(r *http.Request, w http.ResponseWriter) error {
	reqParam := &createReq{}
	if err := json.NewDecoder(r.Body).Decode(reqParam); err != nil {
		return err
	}

	if reqParam.Name == "" {
		return fmt.Errorf("create %s volume driver with no name", driverName)
	}
	if reqParam.Opts == nil || len(reqParam.Opts) == 0 || reqParam.Opts["mount"] == "" {
		return fmt.Errorf("mount and size are required")
	}

	stLock.Lock()
	defer stLock.Unlock()

	if _, exist := store[reqParam.Name]; exist {
		logrus.Infof("volume name %s repeat", reqParam.Name)
		return WriteHttpJson(w, http.StatusOK, &createResp{})
	}

	v := &vol{
		Name:       reqParam.Name,
		MountPoint: reqParam.Opts["mount"],
		Status:     reqParam.Opts,
	}

	if m, err := os.Stat(v.MountPoint); err != nil {
		if e := os.MkdirAll(v.MountPoint, 0644); e != nil {
			return e
		}
	} else if !m.IsDir() {
		return fmt.Errorf("mount path %s is not dir", v.MountPoint)
	}

	store[reqParam.Name] = v

	return WriteHttpJson(w, http.StatusOK, &createResp{})
}

func remove(r *http.Request, w http.ResponseWriter) error {
	reqParam := &removeReq{}
	if err := json.NewDecoder(r.Body).Decode(reqParam); err != nil {
		return err
	}

	if reqParam.Name == "" {
		return fmt.Errorf("remove volume need name")
	}

	v, exist := store[reqParam.Name]
	if !exist {
		return fmt.Errorf("not have %s volume", reqParam.Name)
	}

	stLock.Lock()
	delete(store, reqParam.Name)
	stLock.Unlock()

	os.RemoveAll(v.MountPoint)
	return WriteHttpJson(w, http.StatusOK, &removeResp{})
}

func mount(r *http.Request, w http.ResponseWriter) error {
	reqParam := &mountReq{}
	if err := json.NewDecoder(r.Body).Decode(reqParam); err != nil {
		return err
	}

	if reqParam.Name == "" || reqParam.ID == "" {
		return fmt.Errorf("mount need name and id")
	}

	v, exist := store[reqParam.Name]
	if !exist {
		return fmt.Errorf("not have %s volume", reqParam.Name)
	}

	ids := strings.Split(v.Status["ids"], ",")
	if len(ids) == 0 && ids[0] == "" {
		ids = []string{}
	}

	has := false
	for _, p := range ids {
		if p == reqParam.ID {
			has = true
			break
		}
	}

	stLock.Lock()
	defer stLock.Unlock()

	if has {
		v.Status["ids"] = strings.Join(append(ids, reqParam.ID), ",")
	}

	return WriteHttpJson(w, http.StatusOK, &mountResp{
		MountPoint: v.MountPoint,
	})

}

func path(r *http.Request, w http.ResponseWriter) error {
	reqParam := &pathReq{}

	if reqParam.Name == "" {
		return fmt.Errorf("need volume name")
	}

	v, exist := store[reqParam.Name]
	if !exist {
		return fmt.Errorf("voluem name %s not exist", reqParam.Name)
	}

	return WriteHttpJson(w, http.StatusOK, &pathResp{
		MountPoint: v.MountPoint,
	})
}

func unmount(r *http.Request, w http.ResponseWriter) error {
	reqParam := &unmountReq{}
	if reqParam.Name == "" || reqParam.ID == "" {
		return fmt.Errorf("need volume name")
	}

	v, exist := store[reqParam.Name]
	if !exist {
		return fmt.Errorf("voluem name %s not exist", reqParam.Name)
	}

	ids := strings.Split(v.Status["ids"], ",")
	idx := -1

	for i, id := range ids {
		if id == reqParam.ID {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("volume %s is not mount by id %s", reqParam.Name, reqParam.ID)
	}

	ids = append(ids[:idx], ids[idx+1:]...)
	v.Status["ids"] = strings.Join(ids, ",")

	return WriteHttpJson(w, http.StatusOK, &pathResp{
		MountPoint: v.MountPoint,
	})
}

func get(r *http.Request, w http.ResponseWriter) error {
	reqParam := &getReq{}

	if reqParam.Name == "" {
		return fmt.Errorf("need volume name")
	}

	v, exist := store[reqParam.Name]
	if !exist {
		return fmt.Errorf("voluem name %s not exist", reqParam.Name)
	}

	return WriteHttpJson(w, http.StatusOK, &getResp{
		Volume: v,
	})

}

func list(r *http.Request, w http.ResponseWriter) error {
	var (
		vols   []*vol
		idx    = 0
		length = len(store)
	)
	stLock.Lock()
	stLock.Unlock()

	vols = make([]*vol, length)
	for _, v := range store {
		vols[idx] = v
		idx++
	}
	return WriteHttpJson(w, http.StatusOK, &listResp{
		Volumes: vols,
	})
}

func capabilities(r *http.Request, w http.ResponseWriter) error {
	return WriteHttpJson(w, http.StatusOK, map[string]map[string]string{"Capabilities": map[string]string{"Scope": "local"}})
}

// helper function and type

func WriteHttpJson(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	return json.NewEncoder(w).Encode(v)
}

func WriteHttpError(w http.ResponseWriter, err error) {
	if w == nil || err == nil {
		logrus.WithFields(logrus.Fields{"error": err, "writer": w}).Error("unexpect http error")
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func ShutDownTrap(cleanup func()) {
	c := make(chan os.Signal, 1)
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
	signal.Notify(c, signals...)

	go func() {
		recvSig := <-c
		logrus.Infof("catch os signal %v", recvSig)
		cleanup()
		os.Exit(0)
	}()
}

type vol struct {
	Name       string
	MountPoint string
	Status     map[string]string
}

type createReq struct {
	Name string
	Opts map[string]string
}

type createResp struct {
	Err error
}

type removeReq struct {
	Name string
}

type removeResp struct {
	Err string
}

type mountReq struct {
	Name string
	ID   string
}

type mountResp struct {
	Err        string
	MountPoint string
}

type pathReq struct {
	Name string
}

type pathResp struct {
	MountPoint string
	Err        string
}

type unmountReq struct {
	Name string
	ID   string
}

type umountResp struct {
	Err string
}

type getReq struct {
	Name string
}

type getResp struct {
	Err    error
	Volume *vol
}

type listResp struct {
	Name    string
	Volumes []*vol
}
