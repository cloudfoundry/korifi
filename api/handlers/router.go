package handlers

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
)

type Handler interface {
	AuthenticatedRoutes() []AuthRoute
	UnauthenticatedRoutes() []Route
}

type Route struct {
	Method      string
	Pattern     string
	HandlerFunc HandlerFunc
}

type AuthRoute struct {
	Method      string
	Pattern     string
	HandlerFunc AuthHandlerFunc
}

type Router struct {
	logger    logr.Logger
	inner     chi.Router
	authInner chi.Router
}

func NewRouterBuilder(logger logr.Logger) *Router {
	mux := chi.NewMux()
	return &Router{
		logger: logger,
		inner:  mux,
	}
}

func (r *Router) UseCommonMiddleware(middlewares ...func(http.Handler) http.Handler) {
	r.inner.Use(middlewares...)
}

func (r *Router) UseAuthMiddleware(middlewares ...func(http.Handler) http.Handler) {
	// r.authInner.Use(middlewares...)
}

func (r *Router) RegisterHandler(name string, handler Handler) {
	for _, route := range handler.UnauthenticatedRoutes() {
		r.inner.Method(route.Method, route.Pattern, NewUnauthenticatedWrapper(r.logger.WithName(name), route.HandlerFunc))
	}
	for _, route := range handler.AuthenticatedRoutes() {
		r.inner.Method(route.Method, route.Pattern, NewAuthenticatedWrapper(r.logger.WithName(name), route.HandlerFunc))
	}
}

func (r *Router) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	r.inner.ServeHTTP(res, req)
}

func URLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
