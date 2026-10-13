package httpx

import (
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
)

type AuthorizeFunc func(access.Permission) gin.HandlerFunc

type RouteDeclaration struct {
	Method     string
	Path       string
	Permission access.Permission
}

// Router makes the permission for every API route explicit at registration
// time. Public and first-run setup routes still declare their access class.
type Router struct {
	engine       *gin.Engine
	prefix       string
	authorize    AuthorizeFunc
	mu           sync.Mutex
	declarations []RouteDeclaration
}

func NewRouter(engine *gin.Engine, authorize AuthorizeFunc) *Router {
	return &Router{engine: engine, authorize: authorize}
}

// APIPrefix is where the API is mounted in production; the dashboard's
// static files are served from every other path on the same origin.
const APIPrefix = "/api"

// NewAPIRouter mounts every registered route under APIPrefix. Declarations
// keep the unprefixed path so they continue to match the OpenAPI document.
func NewAPIRouter(engine *gin.Engine, authorize AuthorizeFunc) *Router {
	return &Router{engine: engine, prefix: APIPrefix, authorize: authorize}
}

func (router *Router) GET(path string, permission access.Permission, handlers ...gin.HandlerFunc) {
	router.handle("GET", path, permission, handlers...)
}

func (router *Router) POST(path string, permission access.Permission, handlers ...gin.HandlerFunc) {
	router.handle("POST", path, permission, handlers...)
}

func (router *Router) PATCH(path string, permission access.Permission, handlers ...gin.HandlerFunc) {
	router.handle("PATCH", path, permission, handlers...)
}

func (router *Router) DELETE(path string, permission access.Permission, handlers ...gin.HandlerFunc) {
	router.handle("DELETE", path, permission, handlers...)
}

func (router *Router) handle(method, path string, permission access.Permission, handlers ...gin.HandlerFunc) {
	router.mu.Lock()
	router.declarations = append(router.declarations, RouteDeclaration{
		Method: method, Path: path, Permission: permission,
	})
	router.mu.Unlock()

	if permission != access.Public && permission != access.Setup {
		handlers = append([]gin.HandlerFunc{router.authorize(permission)}, handlers...)
	}
	router.engine.Handle(method, router.prefix+path, handlers...)
}

func (router *Router) Declarations() []RouteDeclaration {
	router.mu.Lock()
	defer router.mu.Unlock()
	result := make([]RouteDeclaration, len(router.declarations))
	copy(result, router.declarations)
	return result
}

func (router *Router) NoRoute(handler gin.HandlerFunc) {
	router.engine.NoRoute(handler)
}
