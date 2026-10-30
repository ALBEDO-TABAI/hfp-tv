package teamserver

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/skillhub"
	"infinite-canvas/backend/internal/skillmarket"

	"github.com/gin-gonic/gin"
)

// MarketBasePath is where the SkillHub v1 contract is served on the internal
// market listener; kernels receive "http://127.0.0.1:<port>" + MarketBasePath.
const MarketBasePath = "/skillhub/v1"

const marketUserKey = "teamserver.market-user"

func marketOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func marketFail(c *gin.Context, err error) {
	typed := skillmarket.AsError(err)
	c.AbortWithStatusJSON(typed.Status, gin.H{"success": false, "reason": typed.Reason, "message": typed.Message})
}

func marketRaw(c *gin.Context, data []byte, contentType, name string) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	c.Data(http.StatusOK, contentType, data)
}

func bearer(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func intQuery(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.Query(name))
	if err != nil {
		return fallback
	}
	return value
}

// MarketHandler serves the SkillHub v1 contract for workspace kernels. It must
// only be bound to a loopback listener: reads are anonymous by contract.
func MarketHandler(market *skillmarket.Service) http.Handler {
	engine := gin.New()
	engine.Use(gin.Recovery())
	api := engine.Group(MarketBasePath)
	member := func(c *gin.Context) {
		user, err := market.Authenticate(c.Request.Context(), bearer(c))
		if err != nil {
			marketFail(c, err)
			return
		}
		c.Set(marketUserKey, user)
		c.Next()
	}
	user := func(c *gin.Context) *accounts.User { return c.MustGet(marketUserKey).(*accounts.User) }

	api.GET("/search", func(c *gin.Context) {
		result, err := market.Search(c.Request.Context(), c.Query("q"), c.Query("target"), c.QueryArray("capability"), intQuery(c, "page", 1), intQuery(c, "pageSize", 20))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/catalog", func(c *gin.Context) {
		result, err := market.Catalog(c.Request.Context(), c.Query("q"), intQuery(c, "page", 1), intQuery(c, "pageSize", 20))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/skills/:skillId/releases/:releaseId", func(c *gin.Context) {
		release, err := market.Release(c.Request.Context(), c.Param("skillId"), c.Param("releaseId"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, release)
	})
	api.GET("/skills/:skillId/releases/:releaseId/discovery", func(c *gin.Context) {
		release, err := market.Release(c.Request.Context(), c.Param("skillId"), c.Param("releaseId"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, skillhub.DiscoveryRelease{Release: *release, Compatibility: skillmarket.Compatibility(release.Requirements, c.QueryArray("capability")), Verification: "unverified"})
	})
	api.GET("/skills/:skillId/releases/:releaseId/file", func(c *gin.Context) {
		name := c.Query("path")
		data, err := market.ReleaseFile(c.Request.Context(), c.Param("skillId"), c.Param("releaseId"), name)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketRaw(c, data, "application/octet-stream", name[strings.LastIndex(name, "/")+1:])
	})
	api.GET("/skills/:skillId/releases/:releaseId/archive", func(c *gin.Context) {
		data, err := market.ReleaseArchive(c.Request.Context(), c.Param("skillId"), c.Param("releaseId"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketRaw(c, data, "application/zip", c.Param("skillId")+"-"+c.Param("releaseId")+".zip")
	})

	api.POST("/authorizations", func(c *gin.Context) {
		var body struct {
			ClientName string `json:"clientName"`
		}
		_ = c.ShouldBindJSON(&body)
		result, err := market.StartAuthorization(c.Request.Context(), body.ClientName)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.POST("/authorizations/token", func(c *gin.Context) {
		var body struct {
			DeviceCode string `json:"deviceCode"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			marketFail(c, skillmarket.ErrAuthorizationExpired)
			return
		}
		result, err := market.ExchangeDeviceCode(c.Request.Context(), body.DeviceCode)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.POST("/authorizations/revoke", func(c *gin.Context) {
		if err := market.RevokeToken(c.Request.Context(), bearer(c)); err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, gin.H{"revoked": true})
	})

	api.GET("/session", member, func(c *gin.Context) {
		result, err := market.Session(c.Request.Context(), user(c))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/submissions", member, func(c *gin.Context) {
		result, err := market.Submissions(c.Request.Context(), user(c))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, gin.H{"items": result})
	})
	api.POST("/submissions", member, func(c *gin.Context) {
		var metadata skillhub.Metadata
		if err := c.ShouldBindJSON(&metadata); err != nil {
			marketFail(c, &skillmarket.Error{Status: http.StatusBadRequest, Reason: "metadata_invalid", Message: "投稿资料格式错误"})
			return
		}
		result, err := market.CreateSubmission(c.Request.Context(), user(c), c.GetHeader("Idempotency-Key"), metadata)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/submissions/:id", member, func(c *gin.Context) {
		result, err := market.Submission(c.Request.Context(), user(c), c.Param("id"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.POST("/submissions/:id/uploads", member, func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, skillhub.MaxArchiveBytes+(1<<20))
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			marketFail(c, &skillmarket.Error{Status: http.StatusBadRequest, Reason: "archive_invalid", Message: "请上传技能 ZIP"})
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, skillhub.MaxArchiveBytes+1))
		if err != nil {
			marketFail(c, &skillmarket.Error{Status: http.StatusBadRequest, Reason: "archive_invalid", Message: "技能 ZIP 读取失败"})
			return
		}
		revision, err := strconv.ParseInt(c.Request.FormValue("revision"), 10, 64)
		if err != nil {
			marketFail(c, skillmarket.ErrRevisionConflict)
			return
		}
		result, err := market.Upload(c.Request.Context(), user(c), c.Param("id"), revision, raw)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.POST("/submissions/:id/validate", member, func(c *gin.Context) {
		var body struct {
			Revision int64 `json:"revision"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			marketFail(c, skillmarket.ErrRevisionConflict)
			return
		}
		result, err := market.Validate(c.Request.Context(), user(c), c.Param("id"), body.Revision)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/submissions/:id/preview", member, func(c *gin.Context) {
		result, err := market.Preview(c.Request.Context(), user(c), c.Param("id"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	api.GET("/submissions/:id/archive", member, func(c *gin.Context) {
		data, err := market.SubmissionArchive(c.Request.Context(), user(c), c.Param("id"))
		if err != nil {
			marketFail(c, err)
			return
		}
		marketRaw(c, data, "application/zip", c.Param("id")+".zip")
	})
	api.POST("/submissions/:id/submit", member, func(c *gin.Context) {
		var body struct {
			Revision      int64  `json:"revision"`
			PreviewDigest string `json:"previewDigest"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			marketFail(c, skillmarket.ErrPreviewChanged)
			return
		}
		result, err := market.Submit(c.Request.Context(), user(c), c.Param("id"), c.GetHeader("Idempotency-Key"), body.Revision, body.PreviewDigest)
		if err != nil {
			marketFail(c, err)
			return
		}
		marketOK(c, result)
	})
	engine.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "reason": "not_found", "message": "请求不存在"})
	})
	return engine
}

// platformFail renders market errors in the platform envelope.
func platformFail(c *gin.Context, err error) {
	if _, ok := err.(*accounts.Error); ok {
		fail(c, err)
		return
	}
	typed := skillmarket.AsError(err)
	failWith(c, typed.Status, typed.Reason, typed.Message)
}

func (s *Server) marketRoutes(platform *gin.RouterGroup, admin *gin.RouterGroup) {
	market := s.opts.Market
	ready := func(c *gin.Context) {
		if market == nil {
			failWith(c, http.StatusServiceUnavailable, "market_not_configured", "未配置技能市场")
			return
		}
		c.Next()
	}
	activeUser := func(c *gin.Context) *accounts.User {
		principal, _ := s.principal(c)
		user := principal.User
		return &user
	}
	members := platform.Group("/market", s.requireSession, ready)
	members.GET("/authorizations/:userCode", func(c *gin.Context) {
		info, err := market.AuthorizationInfo(c.Request.Context(), c.Param("userCode"))
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, info)
	})
	members.POST("/authorizations/:userCode/approve", func(c *gin.Context) {
		if err := market.ApproveAuthorization(c.Request.Context(), activeUser(c).ID, c.Param("userCode")); err != nil {
			platformFail(c, err)
			return
		}
		ok(c, gin.H{"approved": true})
	})

	reviews := admin.Group("", ready)
	reviews.GET("/skill-submissions", func(c *gin.Context) {
		items, err := market.ReviewQueue(c.Request.Context(), c.Query("status"))
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	reviews.GET("/skill-submissions/:id", func(c *gin.Context) {
		detail, err := market.ReviewDetail(c.Request.Context(), c.Param("id"))
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, detail)
	})
	reviews.GET("/skill-submissions/:id/file", func(c *gin.Context) {
		content, err := market.ReviewFile(c.Request.Context(), c.Param("id"), c.Query("path"))
		if err != nil {
			platformFail(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, content)
	})
	reviews.POST("/skill-submissions/:id/approve", func(c *gin.Context) {
		var body struct {
			Revision      int64  `json:"revision"`
			PreviewDigest string `json:"previewDigest"`
			Official      bool   `json:"official"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			platformFail(c, skillmarket.ErrSubmissionChanged)
			return
		}
		view, release, err := market.Approve(c.Request.Context(), activeUser(c), c.Param("id"), body.Revision, body.PreviewDigest, body.Official)
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, gin.H{"submission": view, "release": gin.H{"skillId": release.SkillID, "releaseId": release.ReleaseID}})
	})
	reviews.POST("/skill-submissions/:id/reject", func(c *gin.Context) {
		var body struct {
			Revision int64  `json:"revision"`
			Reason   string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			platformFail(c, skillmarket.ErrReasonRequired)
			return
		}
		view, err := market.Reject(c.Request.Context(), activeUser(c), c.Param("id"), body.Revision, body.Reason)
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, gin.H{"submission": view})
	})
	reviews.GET("/market-skills", func(c *gin.Context) {
		items, err := market.Skills(c.Request.Context())
		if err != nil {
			platformFail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	setListed := func(listed bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			view, err := market.SetListed(c.Request.Context(), activeUser(c), c.Param("skillId"), listed)
			if err != nil {
				platformFail(c, err)
				return
			}
			ok(c, gin.H{"skill": view})
		}
	}
	reviews.POST("/market-skills/:skillId/unlist", setListed(false))
	reviews.POST("/market-skills/:skillId/relist", setListed(true))
}
