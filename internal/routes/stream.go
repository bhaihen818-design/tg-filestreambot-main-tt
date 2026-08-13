package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LoadHome intentionally retains the legacy path only as an explicit terminal
// response. Historical Render links cannot fall back to byte delivery: the
// GDPlayer cloud gateway is the only media data path.
func (e *allRoutes) LoadHome(r *Route) {
	r.Engine.GET("/stream/:messageID", streamMoved)
	r.Engine.HEAD("/stream/:messageID", streamMoved)
}

func streamMoved(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusGone, gin.H{
		"error": "Telegram media streaming has moved to GDPlayer Cloud Hosting. This service never transfers video bytes.",
	})
}
