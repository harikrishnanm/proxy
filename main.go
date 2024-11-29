package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		log.Printf("Error during NewRequest() %s: %s\n", r.URL.String(), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error during Do() %s: %s\n", r.URL.String(), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error during Copy() %s: %s\n", r.URL.String(), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("%s - %s - %s - %d - %dKB\n", r.Proto, r.Method, r.Host, resp.StatusCode, written/1000)
}

func handleTunnel(w http.ResponseWriter, r *http.Request) {
	dest_conn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer dest_conn.Close()
	w.WriteHeader(http.StatusOK)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	src_conn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer src_conn.Close()

	srcConnStr := fmt.Sprintf("%s->%s", src_conn.LocalAddr().String(), src_conn.RemoteAddr().String())
	dstConnStr := fmt.Sprintf("%s->%s", dest_conn.LocalAddr().String(), dest_conn.RemoteAddr().String())

	log.Printf("%s - %s - %s\n", r.Proto, r.Method, r.Host)
	log.Printf("src_conn: %s - dst_conn: %s\n", srcConnStr, dstConnStr)

	var wg sync.WaitGroup

	wg.Add(2)
	go transfer(&wg, dest_conn, src_conn, dstConnStr, srcConnStr)
	go transfer(&wg, src_conn, dest_conn, srcConnStr, dstConnStr)
	wg.Wait()
}

func transfer(wg *sync.WaitGroup, destination io.Writer, source io.Reader, destName, srcName string) {
	defer wg.Done()
	written, err := io.Copy(destination, source)
	if err != nil {
		fmt.Printf("Error during copy from %s to %s: %v\n", srcName, destName, err)
	}
	log.Printf("copied %d bytes from %s to %s\n", written, srcName, destName)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	log.Println("health check")
	fmt.Fprintf(w, "OK")
}

func main() {
	log.Fatal(http.ListenAndServe(
		":8888",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleTunnel(w, r)
			} else {
				if r.URL.Path == "/health" {
					healthCheck(w, r)
				} else {
					handleHTTP(w, r)
				}
			}
		})),
	)
}
