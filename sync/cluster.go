package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juicedata/juicesync/config"
	"github.com/juicedata/juicesync/object"
)

type Stat struct {
	Copied      int64
	CopiedBytes int64
	Failed      int64
	Deleted     int64
}

func updateStats(r *Stat) {
	atomic.AddInt64(&copied, r.Copied)
	atomic.AddInt64(&copiedBytes, r.CopiedBytes)
	atomic.AddInt64(&failed, r.Failed)
	atomic.AddInt64(&deleted, r.Deleted)
}

func httpRequest(url string, body []byte) (ans []byte, err error) {
	method := "GET"
	if body != nil {
		method = "POST"
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func sendStats(addr string) {
	var r Stat
	r.Copied = atomic.LoadInt64(&copied)
	r.CopiedBytes = atomic.LoadInt64(&copiedBytes)
	r.Failed = atomic.LoadInt64(&failed)
	r.Deleted = atomic.LoadInt64(&deleted)
	d, _ := json.Marshal(r)
	ans, err := httpRequest(fmt.Sprintf("http://%s/stats", addr), d)
	if err != nil || string(ans) != "OK" {
		logger.Errorf("update stats: %s %s", string(ans), err)
	} else {
		atomic.AddInt64(&copied, -r.Copied)
		atomic.AddInt64(&copiedBytes, -r.CopiedBytes)
		atomic.AddInt64(&failed, -r.Failed)
		atomic.AddInt64(&deleted, -r.Deleted)
	}
}

func startManager(tasks chan *object.Object) string {
	http.HandleFunc("/fetch", func(w http.ResponseWriter, req *http.Request) {
		var objs []*object.Object
		obj, ok := <-tasks
		if !ok {
			w.Write([]byte("[]"))
			return
		}
		objs = append(objs, obj)
	LOOP:
		for {
			select {
			case obj = <-tasks:
				if obj == nil {
					break LOOP
				}
				objs = append(objs, obj)
				if len(objs) > 100 {
					break LOOP
				}
			default:
				break LOOP
			}
		}
		d, err := json.MarshalIndent(objs, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		logger.Infof("send %d objects to %s", len(objs), req.RemoteAddr)
		w.Write(d)
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			http.Error(w, "POST required", http.StatusBadRequest)
			return
		}
		d, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("read: %s", err)
			return
		}
		var r Stat
		err = json.Unmarshal(d, &r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updateStats(&r)
		w.Write([]byte("OK"))
	})
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		logger.Fatalf("listen: %s", err)
	}
	logger.Infof("Listen at %s", l.Addr())
	go http.Serve(l, nil)
	return l.Addr().String()
}

func findSelfPath() string {
	program := os.Args[0]
	if strings.Contains(program, "/") {
		path, err := filepath.Abs(program)
		if err != nil {
			logger.Fatalf("resolve path %s: %s", program, err)
		}
		return path
	}
	for _, searchPath := range strings.Split(os.Getenv("PATH"), ":") {
		if searchPath != "" {
			p := filepath.Join(searchPath, program)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	logger.Fatalf("can't find path for %s", program)
	panic("")
}

func launchWorker(address string, config *config.Config, wg *sync.WaitGroup) {
	for _, host := range config.Workers {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			// copy
			path := findSelfPath()
			rpath := "/tmp/juicesync"
			cmd := exec.Command("rsync", "-au", path, host+":"+rpath)
			err := cmd.Run()
			if err != nil {
				logger.Errorf("copy to %s: %s", host, err)
				return
			}
			// launch juicesync
			var args = []string{"ssh", host}
			args = append(args, os.Args...)
			args[2] = rpath
			args = append(args, "-manager")
			args = append(args, address)
			cmd = exec.Command("juicesync")
			err = cmd.Start()
			if err != nil {
				logger.Errorf("start juicesync at %s: %s", host, err)
				return
			}
			err = cmd.Wait()
			if err != nil {
				logger.Errorf("%s: %s", host, err)
			}
		}(host)
	}
}

func fetchJobs(todo chan *object.Object, config *config.Config) {
	for {
		// fetch jobs
		url := fmt.Sprintf("http://%s/fetch", config.Manager)
		ans, err := httpRequest(url, nil)
		if err != nil {
			logger.Errorf("fetch jobs: %s", err)
			time.Sleep(time.Second)
			continue
		}
		var jobs []*object.Object
		err = json.Unmarshal(ans, &jobs)
		if err != nil {
			logger.Errorf("Unmarshal %s: %s", string(ans), err)
			time.Sleep(time.Second)
			continue
		}
		logger.Infof("got %d jobs", len(jobs))
		if len(jobs) == 0 {
			break
		}
		for _, obj := range jobs {
			todo <- obj
		}
	}
	close(todo)
}
