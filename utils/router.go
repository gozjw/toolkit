package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

type Ctx struct {
	W       http.ResponseWriter
	R       *http.Request
	aborted bool

	ID     string
	params []Param
}

func (c *Ctx) Abort() {
	c.aborted = true
}

func (c *Ctx) JSON(status int, v any) error {
	c.W.Header().Set("Content-Type", "application/json;charset=utf-8")
	c.W.WriteHeader(status)
	return json.NewEncoder(c.W).Encode(v)
}

type Param struct {
	key   string
	value string
}

func (c *Ctx) GetParam(key string) string {
	for _, item := range c.params {
		if item.key == key {
			return item.value
		}
	}
	return ""
}

func (c *Ctx) SetParam(key, val string) {
	for i := range c.params {
		if c.params[i].key == key {
			c.params[i].value = val
			return
		}
	}
	c.params = append(c.params, Param{key: key, value: val})
}

type Rsp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
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

type Router struct {
	routes  []*Route
	ctxPool sync.Pool
	root    *Group
}

type Group struct {
	router   *Router
	prefix   string
	handlers []HandlerFunc
}

type HandlerFunc func(ctx *Ctx)

type Route struct {
	method   string
	path     string
	segments []string
	handlers []HandlerFunc
}

func NewRouter() *Router {
	r := &Router{
		ctxPool: sync.Pool{New: func() any {
			return &Ctx{}
		}},
	}
	r.root = &Group{
		router: r,
		prefix: "",
	}
	return r
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := ro.ctxPool.Get().(*Ctx)
	c.W, c.R = w, r
	c.aborted = false
	c.params = c.params[:0]
	defer ro.ctxPool.Put(c)

	defer func() {
		if e := recover(); e != nil {
			_ = c.JSON(http.StatusInternalServerError, Rsp{
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
	reqSeg := splitPath(r.URL.Path)

	var hitRoute *Route
	for _, rt := range ro.routes {
		if rt.method != reqMethod {
			continue
		}
		params, ok := rt.match(reqSeg)
		if ok {
			c.params = append(c.params, params...)
			hitRoute = rt
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
		_ = c.JSON(http.StatusNotFound, Rsp{Code: 404, Msg: "not found"})
		return
	}

	for _, handler := range hitRoute.handlers {
		handler(c)
		if c.aborted {
			break
		}
	}
}

func (r *Route) match(reqSegs []string) ([]Param, bool) {
	var params []Param
	if len(r.segments) != len(reqSegs) {
		return params, false
	}
	for i, tpl := range r.segments {
		req := reqSegs[i]
		if paramName, ok := strings.CutPrefix(tpl, "/:"); ok {
			val := req[1:]
			if len(val) == 0 {
				return params, false
			}
			decoded, err := url.PathUnescape(val)
			if err != nil || strings.ContainsAny(decoded, "/\\") {
				return params, false
			}
			params = append(params, Param{
				key:   paramName,
				value: decoded,
			})
			continue
		}
		if tpl != req {
			return params, false
		}
	}
	return params, true
}

func (g *Group) Group(prefix string, mws ...HandlerFunc) *Group {
	fullPre := joinPath(g.prefix, prefix)
	newGroup := &Group{
		router:   g.router,
		prefix:   fullPre,
		handlers: append(append([]HandlerFunc(nil), g.handlers...), mws...),
	}
	return newGroup
}

func (g *Group) Use(mws ...HandlerFunc) {
	g.handlers = append(g.handlers, mws...)
}

func (r *Route) segScore() int {
	s := 0
	for _, seg := range r.segments {
		if strings.HasPrefix(seg, "/:") {
			s++
		}
	}
	return s
}

func (g *Group) addRoute(method, path string, h HandlerFunc) {
	fullPath := joinPath(g.prefix, path)
	segs := splitPath(fullPath)

	handlers := make([]HandlerFunc, 0, len(g.handlers)+1)
	handlers = append(handlers, g.handlers...)
	handlers = append(handlers, h)

	rt := &Route{
		method:   method,
		path:     fullPath,
		segments: segs,
		handlers: handlers,
	}
	score := rt.segScore()

	idx := 0
	routeList := g.router.routes
	for i := range slices.Backward(routeList) {
		existScore := routeList[i].segScore()
		if existScore <= score {
			idx = i + 1
			break
		}
	}

	routeList = append(routeList[:idx], append([]*Route{rt}, routeList[idx:]...)...)
	g.router.routes = routeList
}

func (g *Group) GET(path string, h HandlerFunc) {
	g.addRoute(http.MethodGet, path, h)
}
func (g *Group) POST(path string, h HandlerFunc) {
	g.addRoute(http.MethodPost, path, h)
}
func (g *Group) PUT(path string, h HandlerFunc) {
	g.addRoute(http.MethodPut, path, h)
}
func (g *Group) DELETE(path string, h HandlerFunc) {
	g.addRoute(http.MethodDelete, path, h)
}

func (ro *Router) GET(path string, h HandlerFunc) {
	ro.root.GET(path, h)
}
func (ro *Router) POST(path string, h HandlerFunc) {
	ro.root.POST(path, h)
}
func (ro *Router) PUT(path string, h HandlerFunc) {
	ro.root.PUT(path, h)
}
func (ro *Router) DELETE(path string, h HandlerFunc) {
	ro.root.DELETE(path, h)
}
func (ro *Router) Use(mws ...HandlerFunc) {
	ro.root.Use(mws...)
}
func (ro *Router) Group(prefix string, mws ...HandlerFunc) *Group {
	return ro.root.Group(prefix, mws...)
}

func joinPath(a, b string) string {
	if b == "" {
		if a == "" {
			return "/"
		}
		return a
	}
	a = strings.TrimPrefix(a, "/")
	b = strings.TrimPrefix(b, "/")
	b = strings.TrimSuffix(b, "/")
	return a + "/" + b
}

func splitPath(p string) []string {
	if p == "/" {
		return []string{"/"}
	}
	var segs []string
	for s := range strings.SplitSeq(p, "/") {
		if s == "" {
			continue
		}
		segs = append(segs, "/"+s)
	}
	return segs
}
