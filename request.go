package main

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func requestFor(r *http.Request, target *url.URL) *http.Request {
	req := *r
	u := *req.URL
	u.Scheme = target.Scheme
	u.Host = target.Host
	u.Path = joinPath(target.Path, u.Path)
	if target.RawQuery == "" || u.RawQuery == "" {
		u.RawQuery = target.RawQuery + u.RawQuery
	} else {
		u.RawQuery = target.RawQuery + "&" + u.RawQuery
	}
	req.URL = &u

	req.Proto = "HTTP/1.1"
	req.ProtoMajor = 1
	req.ProtoMinor = 1
	req.Close = false
	req.Host = req.URL.Host

	req.Header = make(http.Header)
	copyHeader(req.Header, r.Header)
	for _, h := range hopHeaders {
		req.Header.Del(h)
	}

	ip, port, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return &req
	}

	if prior, ok := req.Header["X-Forwarded-For"]; ok {
		ip = strings.Join(prior, ", ") + ", " + ip
	}
	req.Header.Set("X-Forwarded-For", ip)

	if req.Header.Get("X-Forwarded-Port") == "" {
		req.Header.Set("X-Forwarded-Port", port)
	}
	return &req
}

func joinPath(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
