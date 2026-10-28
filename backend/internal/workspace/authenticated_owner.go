package workspace

import "context"

type authenticatedOwnerKey struct{}

// WithAuthenticatedOwner is only for the in-process gateway, after it has
// authenticated the session and selected the owner's workspace server-side.
// HTTP headers, cookies and query parameters must never populate this key.
func WithAuthenticatedOwner(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, authenticatedOwnerKey{}, workspaceID)
}

func IsAuthenticatedOwner(ctx context.Context, workspaceID string) bool {
	id, _ := ctx.Value(authenticatedOwnerKey{}).(string)
	return workspaceID != "" && id == workspaceID
}
