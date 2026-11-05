package handler

import (
	"context"

	"infinite-canvas/backend/internal/app"
	"infinite-canvas/backend/internal/generation"
	"infinite-canvas/backend/internal/model"

	"github.com/gin-gonic/gin"
)

type companyReferenceAssetPort interface {
	SyncResourceToArkPrivateAsset(context.Context, *model.User, string) (*generation.ArkPrivateAssetSyncResult, error)
}

// Only hosted workspaces receive this route; the service resolves the managed
// company channel and checks the resource owner before contacting the relay.
func RegisterCompanyReferenceAssetRoutes(r *gin.RouterGroup, svc *app.Service, assets companyReferenceAssetPort) {
	r.POST("/resources/:id/ark-private-asset", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := assets.SyncResourceToArkPrivateAsset(c.Request.Context(), user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"sync": result})
	})
}
