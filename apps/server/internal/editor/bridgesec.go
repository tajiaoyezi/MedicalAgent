package editor

import "medoffice/server/internal/docperm"

// 复刻 services/bridge-security.ts。
type BridgeMethodCategory string

const (
	CatRead         BridgeMethodCategory = "read"
	CatWrite        BridgeMethodCategory = "write"
	CatWriteComment BridgeMethodCategory = "write_comment"
	CatPanel        BridgeMethodCategory = "panel"
)

var readMethods = map[string]bool{
	"getCurrentDocument": true, "getDocumentId": true, "getDocumentTitle": true,
	"getDocumentType": true, "getFullText": true, "getSelectedText": true,
	"getCurrentParagraph": true, "getDocumentOutline": true, "getCurrentPage": true,
	"getComments": true, "getReferences": true,
}

var writeCommentMethods = map[string]bool{"insertComment": true}

var writeMethods = map[string]bool{
	"replaceSelection": true, "insertText": true, "appendSection": true,
	"insertCitation": true, "applyStyle": true, "createNewDocument": true,
	"createPresentation": true, "saveDocument": true,
}

var panelMethods = map[string]bool{
	"openAIPanel": true, "closeAIPanel": true, "runAIPanelSkill": true, "streamContentToEditor": true,
}

// CategorizeBridgeMethod 返回方法类别；未知方法返回 ("", false)。
func CategorizeBridgeMethod(method string) (BridgeMethodCategory, bool) {
	switch {
	case readMethods[method]:
		return CatRead, true
	case writeCommentMethods[method]:
		return CatWriteComment, true
	case writeMethods[method]:
		return CatWrite, true
	case panelMethods[method]:
		return CatPanel, true
	default:
		return "", false
	}
}

func MinPermissionForBridge(cat BridgeMethodCategory) docperm.Level {
	if cat == CatRead || cat == CatPanel {
		return docperm.View
	}
	if cat == CatWriteComment {
		return docperm.Comment
	}
	return docperm.Edit
}

var levelOrder = map[docperm.Level]int{
	docperm.None: 0, docperm.View: 1, docperm.Comment: 2, docperm.Edit: 3, docperm.Manage: 4, docperm.Owner: 5,
}

func IsBridgeMethodAllowed(cat BridgeMethodCategory, level docperm.Level) bool {
	return levelOrder[level] >= levelOrder[MinPermissionForBridge(cat)]
}

func IsTextExportMethod(method string) bool {
	return method == "getFullText" || method == "getSelectedText" || method == "getCurrentParagraph"
}

func RequiresRevisionCheck(method string) bool {
	return writeMethods[method]
}
