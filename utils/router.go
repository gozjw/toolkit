package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type Ctx struct {
	W      http.ResponseWriter
	R      *http.Request
	ID     string
	Params map[string]string
}

func (c *Ctx) JSON(status int, v any) error {
	c.W.Header().Set("Content-Type", "application/json;charset=utf-8")
	c.W.WriteHeader(status)
	return json.NewEncoder(c.W).Encode(v)
}

func (t *Ctx) Log(calldepth int, level string, params ...any) {
	LogImpl.output(2+calldepth, level, t.ID, params...)
}

func (t *Ctx) Logf(calldepth int, level string, format string, params ...any) {
	LogImpl.output(2+calldepth, level, t.ID, fmt.Sprintf(format, params...))
}

func (t *Ctx) Info(params ...any) {
	LogImpl.output(2, "inf", t.ID, params...)
}

func (t *Ctx) Infof(format string, params ...any) {
	LogImpl.output(2, "inf", t.ID, fmt.Sprintf(format, params...))
}

func (t *Ctx) Error(params ...any) {
	LogImpl.output(2, "err", t.ID, params...)
}

func (t *Ctx) Errorf(format string, params ...any) {
	LogImpl.output(2, "err", t.ID, fmt.Sprintf(format, params...))
}

type Rsp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type HandlerFunc func(ctx *Ctx)

type Route struct {
	method   string
	segments []string
	handler  HandlerFunc
}

type Router struct {
	routes []*Route
}

func NewRouter() *Router {
	return &Router{}
}

func (r *Router) splitPath(p string) (segs []string) {
	if p == "/" {
		return []string{"/"}
	}
	parts := strings.Split(p, "/")
	for _, s := range parts {
		if s == "" {
			continue
		}
		segs = append(segs, "/"+s)
	}
	return
}

func (r *Route) match(reqSegs []string) (map[string]string, bool) {
	if len(r.segments) != len(reqSegs) {
		return nil, false
	}
	params := make(map[string]string)

	for i, tpl := range r.segments {
		req := reqSegs[i]

		if after, ok := strings.CutPrefix(tpl, "/:"); ok {
			val := req[1:]
			if len(val) == 0 {
				return nil, false
			}

			decoded, err := url.PathUnescape(val)
			if err != nil {
				return nil, false
			}

			if strings.ContainsAny(decoded, "/\\") {
				return nil, false
			}

			val = decoded
			params[after] = val
			continue
		}
		if tpl != req {
			return nil, false
		}
	}
	return params, true
}

func (r *Router) addRoute(method, path string, h HandlerFunc) {
	newRoute := &Route{
		method:   method,
		segments: r.splitPath(path),
		handler:  h,
	}
	score := newRoute.segScore()

	idx := 0
	for i, rt := range slices.Backward(r.routes) {
		if rt.segScore() <= score {
			idx = i + 1
			break
		}
	}

	r.routes = append(r.routes[:idx], append([]*Route{newRoute}, r.routes[idx:]...)...)
}

func (r *Route) segScore() (s int) {
	for _, v := range r.segments {
		if strings.HasPrefix(v, "/:") {
			s++
		}
	}
	return
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var c = &Ctx{
		W:      w,
		R:      r,
		Params: make(map[string]string),
	}

	defer func() {
		if e := recover(); e != nil {
			c.JSON(http.StatusInternalServerError, Rsp{
				Code: 500,
				Msg:  "server panic",
			})
		}
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-Token")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	reqMethod := r.Method
	reqSeg := ro.splitPath(r.URL.Path)

	var hitRoute *Route

	for _, rt := range ro.routes {
		if rt.method != reqMethod {
			continue
		}
		pm, ok := rt.match(reqSeg)
		if ok {
			hitRoute = rt
			c.Params = pm
			break
		}
	}

	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		c.ID = r.RemoteAddr[:idx]
	} else {
		c.ID = r.RemoteAddr
	}

	if hitRoute == nil {
		c.Error("404", r.URL.Path)
		c.JSON(http.StatusNotFound, Rsp{Code: 404, Msg: "not found"})
		return
	}

	hitRoute.handler(c)
}

func (r *Router) GET(path string, h HandlerFunc) {
	r.addRoute(http.MethodGet, path, h)
}

func (r *Router) POST(path string, h HandlerFunc) {
	r.addRoute(http.MethodPost, path, h)
}

func (r *Router) PUT(path string, h HandlerFunc) {
	r.addRoute(http.MethodPut, path, h)
}

func (r *Router) DELETE(path string, h HandlerFunc) {
	r.addRoute(http.MethodDelete, path, h)
}
