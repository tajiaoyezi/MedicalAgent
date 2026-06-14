package editor

import (
	"strings"

	"medoffice/server/internal/config"
	"medoffice/server/internal/docperm"
)

func BuildDocumentKey(documentID, versionID string) string {
	return documentID + "_" + versionID
}

type BuildConfigInput struct {
	Session      *EditorSession
	Filename     string
	DocumentType OnlyofficeDocumentType
	Permission   docperm.Level
	UserID       string
	DisplayName  string
}

// BuildEditorConfig 复刻 editor-config.ts：构造 ONLYOFFICE 配置并经 JWT 包装，附 bridgeToken/revision/mimeType。
// callbackUrl 仅在需要时写入（与 Node 的 undefined-省略 一致，不写 null）。
func BuildEditorConfig(cfg config.OnlyOffice, j *JWT, in BuildConfigInput) map[string]any {
	ext := "docx"
	if strings.Contains(in.Filename, ".") {
		ext = strings.ToLower(in.Filename[strings.LastIndex(in.Filename, ".")+1:])
	}
	editable := docperm.CanEdit(in.Permission)
	commentable := docperm.CanComment(in.Permission)
	copyable := docperm.CanCopy(in.Permission)

	downloadURL := cfg.APIPublicURL + "/api/editor/download/" + in.Session.OpenToken
	needsCallback := editable || (commentable && !editable)
	mode := "view"
	if needsCallback {
		mode = "edit"
	}

	editorConfig := map[string]any{
		"mode":          mode,
		"lang":          "zh-CN",
		"user":          map[string]any{"id": in.UserID, "name": in.DisplayName},
		"customization": map[string]any{"forcesave": true, "plugins": true},
		"plugins": map[string]any{
			"autostart":   []string{"asc.{medoffice-bridge}"},
			"pluginsData": []string{cfg.PluginURL + "config.json"},
		},
	}
	if needsCallback {
		editorConfig["callbackUrl"] = cfg.APIPublicURL + "/api/editor/callback?token=" + in.Session.CallbackToken
	}

	coreConfig := map[string]any{
		"documentType": in.DocumentType,
		"document": map[string]any{
			"fileType": OnlyofficeFileType(in.DocumentType, ext),
			"key":      in.Session.DocumentKey,
			"title":    in.Filename,
			"url":      downloadURL,
			"permissions": map[string]any{
				"edit":     editable,
				"comment":  commentable && !editable,
				"copy":     copyable,
				"download": true,
				"print":    true,
			},
		},
		"editorConfig": editorConfig,
	}

	wrapped := j.Wrap(coreConfig)
	wrapped["bridgeToken"] = in.Session.BridgeToken
	wrapped["revision"] = in.Session.Revision
	wrapped["mimeType"] = MimeForExtension(ext)
	return wrapped
}
