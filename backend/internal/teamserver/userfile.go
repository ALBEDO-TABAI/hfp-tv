package teamserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"strings"
	"time"

	"infinite-canvas/backend/internal/accounts"
)

// AccountFiles points at the server-side account files: UsersPath holds
// "username:password" lines, the optional KeysPath "username:relay-key" lines.
type AccountFiles struct {
	UsersPath string
	KeysPath  string
}

// Fingerprint identifies the current content of both files.
func (f AccountFiles) Fingerprint() (string, error) {
	hash := sha256.New()
	for _, path := range []string{f.UsersPath, f.KeysPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write(raw)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// WarnLoosePermissions logs when the files are readable by other host users;
// the users file may hold plaintext passwords.
func (f AccountFiles) WarnLoosePermissions() {
	for _, path := range []string{f.UsersPath, f.KeysPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o007 != 0 {
			log.Printf("警告：%s 对其他用户可读（权限 %v），建议 chmod 600 并归属运行账号", path, info.Mode().Perm())
		}
	}
}

// SyncAccountFiles applies the files once, then carries the effects into the
// kernels: changed relay keys are re-provisioned and removed accounts closed.
func (s *Server) SyncAccountFiles(ctx context.Context, files AccountFiles) (*accounts.FileSyncResult, error) {
	entries, err := accounts.ParseAccountFiles(files.UsersPath, files.KeysPath)
	if err != nil {
		return nil, err
	}
	result, err := s.opts.Accounts.SyncFileAccounts(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, workspace := range result.RelayChanged {
		spec, err := s.workspaceSpec(workspace)
		if err == nil {
			err = s.opts.Pool.Reprovision(spec)
		}
		if err != nil {
			// The binding is saved; the next kernel open applies it.
			log.Printf("apply relay key workspace=%s: %v", workspace.ID, err)
		}
	}
	for _, id := range result.DisabledWorkspaces {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		if err := s.opts.Pool.Close(closeCtx, id); err != nil {
			log.Printf("close disabled workspace=%s: %v", id, err)
		}
		cancel()
	}
	return result, nil
}

// WatchAccountFiles re-applies the files when their content changes. A new
// version is applied only after two identical reads, so a file an editor is
// still writing is never synced; a broken file leaves the accounts unchanged
// and is retried until it is fixed.
func (s *Server) WatchAccountFiles(ctx context.Context, files AccountFiles, interval time.Duration, applied string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	pending, lastError := "", ""
	report := func(message string) {
		if message != lastError {
			log.Print(message)
			lastError = message
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		current, err := files.Fingerprint()
		if err != nil {
			report("读取账号文件失败，保持原有账号：" + err.Error())
			continue
		}
		if current == applied {
			pending = ""
			continue
		}
		if current != pending {
			pending = current
			continue
		}
		result, err := s.SyncAccountFiles(ctx, files)
		if err != nil {
			report("账号文件有误，保持原有账号：" + err.Error())
			continue
		}
		applied, pending, lastError = current, "", ""
		LogAccountSync(result)
	}
}

// LogAccountSync prints usernames only; never passwords or keys.
func LogAccountSync(result *accounts.FileSyncResult) {
	if !result.Changed() {
		log.Printf("账号文件已同步，没有变化")
		return
	}
	log.Printf("账号文件已同步：新增 %d %v，更新 %d %v，停用 %d %v", len(result.Created), result.Created,
		len(result.Updated), result.Updated, len(result.Disabled), result.Disabled)
}
