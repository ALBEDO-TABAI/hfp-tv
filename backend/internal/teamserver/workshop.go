package teamserver

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/workshop"

	"github.com/gin-gonic/gin"
)

func workshopFail(c *gin.Context, err error) {
	typed := workshop.AsError(err)
	body := gin.H{"code": typed.Status, "data": nil, "msg": typed.Message, "reason": typed.Reason}
	if typed.Details != nil {
		body["data"] = typed.Details
	}
	c.AbortWithStatusJSON(typed.Status, body)
}

// snapshotKernel opens the viewer's own kernel for export or import.
func (s *Server) snapshotKernel(ctx context.Context, workspace accounts.Workspace) (workshop.SnapshotPort, error) {
	spec, err := s.workspaceSpec(workspace)
	if err != nil {
		return nil, err
	}
	kernel, err := s.opts.Pool.Get(ctx, spec)
	if err != nil {
		return nil, err
	}
	return kernel.ProjectSnapshots()
}

func (s *Server) viewer(c *gin.Context) workshop.Viewer {
	principal, _ := s.principal(c)
	return workshop.Viewer{User: principal.User, Workspace: principal.Workspace, Admin: principal.IsAdmin()}
}

// inlineMedia lists types a browser may render from this origin; anything
// else (HTML, SVG, scripts) is served as a download.
func inlineMedia(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/")
}

func (s *Server) workshopRoutes(admin *gin.RouterGroup) {
	shop := s.opts.Workshop
	ready := func(c *gin.Context) {
		if shop == nil {
			failWith(c, http.StatusServiceUnavailable, "workshop_not_configured", "未启用创意工坊")
			return
		}
		c.Next()
	}
	community := s.engine.Group("/api/community", s.requireSession, ready)
	community.POST("/submissions/preflight", func(c *gin.Context) {
		var body struct {
			ProjectID string `json:"projectId"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ProjectID) == "" {
			failWith(c, http.StatusBadRequest, "project_required", "请选择项目")
			return
		}
		result, err := shop.Preflight(c.Request.Context(), s.viewer(c), body.ProjectID)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, result)
	})
	community.POST("/submissions", func(c *gin.Context) {
		var input workshop.SubmissionInput
		if err := c.ShouldBindJSON(&input); err != nil {
			failWith(c, http.StatusBadRequest, "submission_invalid", "投稿资料格式错误")
			return
		}
		publication, version, err := shop.Submit(c.Request.Context(), s.viewer(c), input)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"publication": publication, "version": version})
	})
	community.GET("/submissions/mine", func(c *gin.Context) {
		items, err := shop.MySubmissions(c.Request.Context(), s.viewer(c))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	community.POST("/versions/:vid/withdraw", func(c *gin.Context) {
		if err := shop.Withdraw(c.Request.Context(), s.viewer(c), c.Param("vid")); err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"withdrawn": true})
	})
	community.GET("/versions/:vid/snapshot", func(c *gin.Context) {
		raw, err := shop.Snapshot(c.Request.Context(), s.viewer(c), c.Param("vid"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		c.Header("Cache-Control", "private, max-age=300")
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(append([]byte(`{"code":0,"msg":"ok","data":`), raw...), '}'))
	})
	community.GET("/files/:sha256", func(c *gin.Context) {
		access, err := shop.File(c.Request.Context(), s.viewer(c), c.Query("v"), c.Param("sha256"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		file, err := os.Open(access.Path)
		if err != nil {
			failWith(c, http.StatusNotFound, "file_not_found", "文件不存在")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			failWith(c, http.StatusNotFound, "file_not_found", "文件不存在")
			return
		}
		contentType := access.MimeType
		disposition := "inline"
		if !inlineMedia(contentType) {
			contentType, disposition = "application/octet-stream", "attachment"
		}
		if c.Query("download") == "1" && access.AllowDownload {
			disposition = "attachment"
		}
		name := strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(access.Name)
		c.Header("Content-Type", contentType)
		c.Header("Content-Disposition", disposition+`; filename="`+name+`"`)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
		c.Header("Cache-Control", "private, max-age=3600")
		http.ServeContent(c.Writer, c.Request, "", info.ModTime(), file)
	})
	community.GET("/publications", func(c *gin.Context) {
		page, _ := strconv.Atoi(c.Query("page"))
		size, _ := strconv.Atoi(c.Query("pageSize"))
		items, total, err := shop.List(c.Request.Context(), s.viewer(c), workshop.ListQuery{Query: c.Query("q"), Source: c.Query("source"),
			Sort: c.Query("sort"), Tag: c.Query("tag"), Page: page, PageSize: size})
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"items": items, "total": total})
	})
	community.GET("/publications/:id", func(c *gin.Context) {
		view, err := shop.Publication(c.Request.Context(), s.viewer(c), c.Param("id"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, view)
	})
	community.POST("/publications/:id/copies", func(c *gin.Context) {
		var body struct {
			RequestID string `json:"requestId"`
			Title     string `json:"title"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			workshopFail(c, workshop.ErrRequestInvalid)
			return
		}
		copied, err := shop.Copy(c.Request.Context(), s.viewer(c), c.Param("id"), body.RequestID, body.Title)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"copy": copied})
	})
	community.GET("/copies/:requestId", func(c *gin.Context) {
		copied, err := shop.CopyStatus(c.Request.Context(), s.viewer(c), c.Param("requestId"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"copy": copied})
	})
	community.POST("/publications/:id/unlist", func(c *gin.Context) {
		if err := shop.AuthorUnlist(c.Request.Context(), s.viewer(c), c.Param("id")); err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"unlisted": true})
	})
	community.GET("/origins/:projectId", func(c *gin.Context) {
		origin, err := shop.Origin(c.Request.Context(), s.viewer(c), c.Param("projectId"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, origin)
	})

	reviews := admin.Group("/workshop", ready)
	reviews.GET("/versions", func(c *gin.Context) {
		items, err := shop.ReviewQueue(c.Request.Context(), s.viewer(c), c.Query("status"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	reviews.GET("/versions/:vid", func(c *gin.Context) {
		item, err := shop.ReviewDetail(c.Request.Context(), s.viewer(c), c.Param("vid"))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, item)
	})
	reviews.POST("/versions/:vid/approve", func(c *gin.Context) {
		var body struct {
			SnapshotSHA256 string `json:"snapshotSha256"`
			Official       bool   `json:"official"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			workshopFail(c, workshop.ErrVersionChanged)
			return
		}
		item, err := shop.Approve(c.Request.Context(), s.viewer(c), c.Param("vid"), body.SnapshotSHA256, body.Official)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, item)
	})
	reviews.POST("/versions/:vid/reject", func(c *gin.Context) {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		item, err := shop.Reject(c.Request.Context(), s.viewer(c), c.Param("vid"), body.Reason)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, item)
	})
	reviews.GET("/publications", func(c *gin.Context) {
		items, err := shop.AdminPublications(c.Request.Context(), s.viewer(c))
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	reviews.POST("/publications/:id/:action", func(c *gin.Context) {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		view, err := shop.Moderate(c.Request.Context(), s.viewer(c), c.Param("id"), c.Param("action"), body.Reason)
		if err != nil {
			workshopFail(c, err)
			return
		}
		ok(c, gin.H{"publication": view})
	})
}
