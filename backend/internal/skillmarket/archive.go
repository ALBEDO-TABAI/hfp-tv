package skillmarket

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"infinite-canvas/backend/internal/skillhub"
)

const (
	maxFiles      = 256
	maxFileBytes  = 2 << 20
	maxTotalBytes = 20 << 20
	maxSkillMD    = 256 << 10
)

// canonicalArchive is what the market stores and serves for a submission.
type canonicalArchive struct {
	Data        []byte
	Files       []skillhub.File
	ContentHash string
	Contents    map[string][]byte
}

func mimeTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md":
		return "text/markdown"
	case ".txt", "":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".csv":
		return "text/csv"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return "application/octet-stream"
}

func junkEntry(name string) bool {
	base := path.Base(name)
	return strings.HasPrefix(name, "__MACOSX/") || base == ".DS_Store"
}

// canonicalize validates an uploaded ZIP with the same rules installers apply
// (skillhub.VerifyArchive) and re-packs it deterministically: sorted paths,
// Deflate, mode 0600, no timestamps. Clients compare files, not ZIP bytes.
func canonicalize(raw []byte) (*canonicalArchive, error) {
	if len(raw) == 0 || len(raw) > skillhub.MaxArchiveBytes {
		return nil, newError(http.StatusRequestEntityTooLarge, "archive_too_large", "技能包需小于 10 MiB")
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, invalid("archive_invalid", "上传内容不是有效的 ZIP")
	}
	contents := map[string][]byte{}
	lower := map[string]bool{}
	var total int64
	for _, entry := range reader.File {
		if strings.HasSuffix(entry.Name, "/") || junkEntry(entry.Name) {
			continue
		}
		if !entry.Mode().IsRegular() || entry.Flags&1 != 0 {
			return nil, invalid("archive_invalid", "技能包只能包含普通文件")
		}
		if !skillhub.DataFilePath(entry.Name) {
			return nil, invalid("file_not_allowed", "不允许的文件："+entry.Name+"（只接受文档、数据和图片，不接受脚本）")
		}
		key := strings.ToLower(entry.Name)
		if lower[key] {
			return nil, invalid("archive_invalid", "技能包含重复文件："+entry.Name)
		}
		lower[key] = true
		if entry.UncompressedSize64 > maxFileBytes {
			return nil, invalid("file_too_large", "单个文件需小于 2 MiB："+entry.Name)
		}
		body, err := entry.Open()
		if err != nil {
			return nil, invalid("archive_invalid", "无法读取 ZIP 文件")
		}
		data, err := io.ReadAll(io.LimitReader(body, maxFileBytes+1))
		_ = body.Close()
		if err != nil || len(data) > maxFileBytes {
			return nil, invalid("file_too_large", "单个文件需小于 2 MiB："+entry.Name)
		}
		total += int64(len(data))
		contents[entry.Name] = data
	}
	if len(contents) == 0 || len(contents) > maxFiles {
		return nil, invalid("archive_invalid", "技能包需包含 1–256 个文件")
	}
	if total > maxTotalBytes {
		return nil, invalid("archive_too_large", "技能包文件总大小需小于 20 MiB")
	}
	skill := contents["SKILL.md"]
	if len(skill) == 0 || len(skill) > maxSkillMD || !utf8.Valid(skill) {
		return nil, invalid("skill_md_invalid", "技能包根目录需有 UTF-8 的 SKILL.md（小于 256 KiB）")
	}
	if _, err := skillhub.SkillName(skill); err != nil {
		return nil, invalid("skill_md_invalid", err.Error())
	}
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	files := make([]skillhub.File, 0, len(names))
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(contents[name]); err != nil {
			return nil, err
		}
		files = append(files, skillhub.File{Path: name, SHA256: skillhub.Digest(contents[name]), Size: int64(len(contents[name])), MimeType: mimeTypeFor(name)})
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	archive := &canonicalArchive{Data: buffer.Bytes(), Files: files, ContentHash: skillhub.ContentDigest(contents), Contents: contents}
	// The strict installer check must accept exactly what the market will serve.
	if _, err := skillhub.VerifyArchive(archive.Data, &skillhub.Release{Files: files, ContentHash: archive.ContentHash,
		ArchiveSHA256: skillhub.Digest(archive.Data), ArchiveBytes: int64(len(archive.Data))}); err != nil {
		return nil, invalid("archive_invalid", err.Error())
	}
	return archive, nil
}

// readArchiveFile returns one file from a stored canonical archive.
func readArchiveFile(data []byte, name string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		body, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer body.Close()
		return io.ReadAll(io.LimitReader(body, maxFileBytes+1))
	}
	return nil, ErrNotFound
}
