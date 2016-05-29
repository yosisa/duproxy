package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/tylerb/graceful"
	"github.com/yosisa/webutil"
)

var (
	listen          = flag.String("listen", ":8080", "Listen address")
	gracefulTimeout = flag.Duration("graceful-timeout", 10*time.Second, "Wait until force shutdown")
)

type duproxy struct {
	primary     *url.URL
	secondaries []*url.URL
	transport   http.RoundTripper
	bufferPool  *sync.Pool
}

func newDuproxy(primary string, secondaries ...string) (*duproxy, error) {
	dp := &duproxy{
		transport: http.DefaultTransport,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
	var err error
	if dp.primary, err = url.Parse(primary); err != nil {
		return nil, err
	}
	for _, secondary := range secondaries {
		u, err := url.Parse(secondary)
		if err != nil {
			return nil, err
		}
		dp.secondaries = append(dp.secondaries, u)
	}
	return dp, nil
}

func (d *duproxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mr := NewMultiReader(r.Body)
	for _, secondary := range d.secondaries {
		go func(u *url.URL, body io.ReadCloser) {
			req := requestFor(r, u)
			req.Body = body
			res, err := d.transport.RoundTrip(req)
			if err != nil {
				log.Println(err)
				return
			}
			d.copyResponse(ioutil.Discard, res.Body)
		}(secondary, mr.Reader())
	}

	req := requestFor(r, d.primary)
	req.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: mr,
		Closer: r.Body,
	}

	res, err := d.transport.RoundTrip(req)
	if err != nil {
		log.Println(err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	for _, h := range hopHeaders {
		res.Header.Del(h)
	}
	copyHeader(w.Header(), res.Header)

	w.WriteHeader(res.StatusCode)
	d.copyResponse(w, res.Body)
}

func (d *duproxy) copyResponse(dst io.Writer, src io.ReadCloser) {
	buf := d.bufferPool.Get().([]byte)
	io.CopyBuffer(dst, src, buf)
	d.bufferPool.Put(buf)
	src.Close()
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("invalid arguments")
	}
	dp, err := newDuproxy(args[0], args[1:]...)
	if err != nil {
		log.Fatal(err)
	}
	h := webutil.Logger(dp, webutil.ConsoleLogWriter(os.Stdout))
	h = webutil.Recoverer(h, os.Stderr)
	graceful.Run(*listen, *gracefulTimeout, h)
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage of %s:
  %s [OPTIONS] PRIMARY [SECONDARY...]

Options:
`, os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}
}
