package teamserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/kernelpool"
	"infinite-canvas/backend/internal/skillmarket"
)

func newMarketHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "false")
	root := t.TempDir()
	users, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := skillmarket.LoadSigner(filepath.Join(root, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	market, err := skillmarket.Open(users.DB(), users, signer, root)
	if err != nil {
		t.Fatal(err)
	}
	marketServer := httptest.NewServer(MarketHandler(market))
	t.Setenv("BEEFTV_SKILLHUB_URL", marketServer.URL+MarketBasePath)
	t.Setenv("BEEFTV_SKILLHUB_TRUSTED_KEYS", signer.TrustedKeysJSON())
	t.Setenv("BEEFTV_SKILLHUB_REQUIRE_SIGNATURE", "true")
	pool := kernelpool.New(kernelpool.Options{ShutdownTimeout: 5 * time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = pool.CloseAll(ctx)
		marketServer.Close()
		_ = users.Close()
	})
	server, err := New(Options{Accounts: users, Pool: pool, DataRoot: root, CookieSecure: true, Market: market})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, server: server, users: users}
}

func decodeData(t *testing.T, response *httptest.ResponseRecorder, out any) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || envelope.Code != 0 {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			t.Fatal(err)
		}
	}
}

type marketPlan struct {
	PlanID        string `json:"planId"`
	ArchiveSHA256 string `json:"archiveSha256"`
	Status        string `json:"status"`
	SubmissionID  string `json:"submissionId"`
	ReviewReason  string `json:"reviewReason"`
	Metadata      struct {
		SkillID string `json:"skillId"`
	} `json:"metadata"`
	Preview *struct {
		Revision      int64  `json:"revision"`
		PreviewDigest string `json:"previewDigest"`
	} `json:"preview"`
}

// publishThroughKernel drives the same kernel routes the web publishing panel uses.
func publishThroughKernel(h *harness, cookie, localSkillID, version, target, marketSkillID string) marketPlan {
	h.t.Helper()
	body, _ := json.Marshal(map[string]any{"skillId": localSkillID, "publicationTarget": target, "marketSkillId": marketSkillID,
		"metadata": map[string]any{"name": "分镜助手", "description": "把剧本拆成镜头表", "versionLabel": version, "license": "CC-BY-4.0",
			"sourceUrl": "", "requirements": []string{"beeftv.canvas"}, "showcase": []any{}}})
	var plan marketPlan
	decodeData(h.t, h.do(call{method: http.MethodPost, path: "/api/skills/market/publish-plans", cookie: cookie, body: string(body)}), &plan)
	upload, _ := json.Marshal(map[string]any{"confirmed": true, "archiveSha256": plan.ArchiveSHA256})
	decodeData(h.t, h.do(call{method: http.MethodPost, path: "/api/skills/market/publish-plans/" + plan.PlanID + "/upload", cookie: cookie, body: string(upload)}), &plan)
	if plan.Status != "awaiting_submit_confirmation" || plan.Preview == nil {
		h.t.Fatalf("upload did not reach preview: %+v", plan)
	}
	submit, _ := json.Marshal(map[string]any{"confirmed": true, "revision": plan.Preview.Revision, "previewDigest": plan.Preview.PreviewDigest})
	decodeData(h.t, h.do(call{method: http.MethodPost, path: "/api/skills/market/publish-plans/" + plan.PlanID + "/submit", cookie: cookie, body: string(submit)}), &plan)
	if plan.Status != skillmarket.StatusInReview {
		h.t.Fatalf("submission not in review: %+v", plan)
	}
	return plan
}

func TestInHouseSkillMarketPublishReviewInstall(t *testing.T) {
	h := newMarketHarness(t)
	ctx := context.Background()
	admin, _ := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	_, _ = h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass"})
	_, _ = h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"})
	adminCookie, aliceCookie, bobCookie := h.login("admin", "admin-pass"), h.login("alice", "alice-pass"), h.login("bob", "bob-pass1")

	// Alice writes a skill in her own workspace.
	var created struct {
		Skill struct {
			SkillID string `json:"skillId"`
		} `json:"skill"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/skills", cookie: aliceCookie,
		body: `{"skillName":"分镜助手","description":"把剧本拆成镜头表","instruction":"读取剧本，按场景输出镜头表。","tag":"others","isPrivate":true}`}), &created)

	// Device authorization is confirmed inside the app, never on an external site.
	var started struct {
		UserCode        string `json:"userCode"`
		VerificationURL string `json:"verificationUrl"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/skills/market/authorization/start", cookie: aliceCookie, body: `{}`}), &started)
	if !strings.HasPrefix(started.VerificationURL, skillmarket.VerificationPath+"?code=") || started.UserCode == "" {
		t.Fatalf("verification URL leaves the app: %+v", started)
	}
	var pending struct {
		Connected bool `json:"connected"`
		Pending   bool `json:"pending"`
	}
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/authorization", cookie: aliceCookie}), &pending)
	if pending.Connected || !pending.Pending {
		t.Fatalf("authorization connected before approval: %+v", pending)
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/platform/market/authorizations/" + started.UserCode + "/approve", cookie: aliceCookie, body: `{}`}), nil)
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/authorization", cookie: aliceCookie}), &pending)
	if !pending.Connected {
		t.Fatal("approved authorization did not connect")
	}

	plan := publishThroughKernel(h, aliceCookie, created.Skill.SkillID, "1.0.0", "new", "")

	// Nothing is public before review.
	var search struct {
		Items []struct {
			SkillID   string `json:"skillId"`
			ReleaseID string `json:"releaseId"`
			Author    string `json:"author"`
		} `json:"items"`
	}
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/search?q=", cookie: bobCookie}), &search)
	if len(search.Items) != 0 {
		t.Fatal("unreviewed skill is visible")
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/platform/admin/skill-submissions?status=pending", cookie: aliceCookie}); response.Code != http.StatusForbidden {
		t.Fatalf("member reached review queue: %d", response.Code)
	}

	var queue struct {
		Items []skillmarket.SubmissionView `json:"items"`
	}
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/platform/admin/skill-submissions?status=pending", cookie: adminCookie}), &queue)
	if len(queue.Items) != 1 || queue.Items[0].Author.Username != "alice" || queue.Items[0].Target != skillmarket.TargetNew {
		t.Fatalf("review queue = %+v", queue.Items)
	}
	item := queue.Items[0]
	var skillFile skillmarket.FileContent
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/platform/admin/skill-submissions/" + item.ID + "/file?path=SKILL.md", cookie: adminCookie}), &skillFile)
	if skillFile.Encoding != "utf-8" || !strings.Contains(skillFile.Content, "镜头表") {
		t.Fatalf("reviewer cannot read SKILL.md: %+v", skillFile)
	}
	stale, _ := json.Marshal(map[string]any{"revision": item.Revision, "previewDigest": "stale", "official": false})
	if response := h.do(call{method: http.MethodPost, path: "/api/platform/admin/skill-submissions/" + item.ID + "/approve", cookie: adminCookie, body: string(stale)}); response.Code != http.StatusConflict {
		t.Fatalf("approval of a stale preview = %d", response.Code)
	}
	approve, _ := json.Marshal(map[string]any{"revision": item.Revision, "previewDigest": item.PreviewDigest, "official": false})
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/platform/admin/skill-submissions/" + item.ID + "/approve", cookie: adminCookie, body: string(approve)}), nil)

	// The author's plan reflects the decision through the kernel refresh.
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/skills/market/publish-plans/" + plan.PlanID + "/refresh", cookie: aliceCookie, body: `{}`}), &plan)
	if plan.Status != skillmarket.StatusPublished {
		t.Fatalf("author plan after approval = %+v", plan)
	}

	// Bob finds it, reads it (signature verified by the kernel) and installs it.
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/search?q=分镜", cookie: bobCookie}), &search)
	if len(search.Items) != 1 || search.Items[0].Author != "alice" {
		t.Fatalf("published skill not found: %+v", search.Items)
	}
	release := search.Items[0]
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/releases/" + release.SkillID + "/" + release.ReleaseID, cookie: bobCookie}), nil)
	installBody, _ := json.Marshal(map[string]string{"skillId": release.SkillID, "releaseId": release.ReleaseID})
	var installed struct {
		Skill struct {
			SkillID string `json:"skillId"`
		} `json:"skill"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/skills/market/install", cookie: bobCookie, body: string(installBody)}), &installed)
	if installed.Skill.SkillID == "" {
		t.Fatal("install returned no skill")
	}

	// A second version from Alice is rejected; the reason reaches her plan.
	second := publishThroughKernel(h, aliceCookie, created.Skill.SkillID, "1.1.0", "existing", release.SkillID)
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/platform/admin/skill-submissions?status=pending", cookie: adminCookie}), &queue)
	if len(queue.Items) != 1 || queue.Items[0].Target != skillmarket.TargetExisting {
		t.Fatalf("update not queued: %+v", queue.Items)
	}
	var detail skillmarket.SubmissionDetail
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/platform/admin/skill-submissions/" + queue.Items[0].ID, cookie: adminCookie}), &detail)
	if detail.PreviousRelease == nil || detail.PreviousRelease.ReleaseID != release.ReleaseID {
		t.Fatalf("update review lacks the previous release: %+v", detail.PreviousRelease)
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/platform/admin/skill-submissions/" + queue.Items[0].ID + "/reject", cookie: adminCookie,
		body: `{"revision":` + jsonInt(queue.Items[0].Revision) + `,"reason":""}`}); response.Code != http.StatusBadRequest {
		t.Fatalf("rejection without reason = %d", response.Code)
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/platform/admin/skill-submissions/" + queue.Items[0].ID + "/reject", cookie: adminCookie,
		body: `{"revision":` + jsonInt(queue.Items[0].Revision) + `,"reason":"内容不完整"}`}), nil)
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/skills/market/publish-plans/" + second.PlanID + "/refresh", cookie: aliceCookie, body: `{}`}), &second)
	if second.Status != skillmarket.StatusRejected || second.ReviewReason != "内容不完整" {
		t.Fatalf("rejection not visible to author: %+v", second)
	}

	// Unlisting hides the skill from search without touching installed copies.
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/platform/admin/market-skills/" + release.SkillID + "/unlist", cookie: adminCookie, body: `{}`}), nil)
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/skills/market/search?q=", cookie: bobCookie}), &search)
	if len(search.Items) != 0 {
		t.Fatal("unlisted skill still searchable")
	}
}

func jsonInt(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
