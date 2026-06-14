// Package editor 复刻 c02 ONLYOFFICE 桥：文件路由、JWT、内存会话、编辑器配置、回调处理、桥安全。
package editor

import "strings"

type EditorRoute string

const (
	RouteOnlyoffice   EditorRoute = "onlyoffice"
	RoutePreviewPDF   EditorRoute = "preview-pdf"
	RoutePreviewImage EditorRoute = "preview-image"
	RoutePreviewOFD   EditorRoute = "preview-ofd"
	RouteUnsupported  EditorRoute = "unsupported"
)

type OnlyofficeDocumentType string

var (
	wordExt  = map[string]bool{"docx": true, "doc": true}
	cellExt  = map[string]bool{"xlsx": true, "xls": true}
	slideExt = map[string]bool{"pptx": true, "ppt": true}
	pdfExt   = map[string]bool{"pdf": true}
	imageExt = map[string]bool{"png": true, "jpg": true, "jpeg": true}
	ofdExt   = map[string]bool{"ofd": true}
)

func ExtensionOf(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return ""
	}
	return strings.ToLower(filename[dot+1:])
}

type RouteInfo struct {
	Route        EditorRoute
	DocumentType OnlyofficeDocumentType // 仅 onlyoffice/pdf 路由有
	FileType     string
}

func ResolveEditorRoute(filename string) RouteInfo {
	ext := ExtensionOf(filename)
	switch {
	case wordExt[ext]:
		return RouteInfo{Route: RouteOnlyoffice, DocumentType: "word", FileType: ext}
	case cellExt[ext]:
		return RouteInfo{Route: RouteOnlyoffice, DocumentType: "cell", FileType: ext}
	case slideExt[ext]:
		return RouteInfo{Route: RouteOnlyoffice, DocumentType: "slide", FileType: ext}
	case pdfExt[ext]:
		return RouteInfo{Route: RoutePreviewPDF, DocumentType: "pdf", FileType: ext}
	case imageExt[ext]:
		return RouteInfo{Route: RoutePreviewImage, FileType: ext}
	case ofdExt[ext]:
		return RouteInfo{Route: RoutePreviewOFD, FileType: ext}
	default:
		return RouteInfo{Route: RouteUnsupported, FileType: ext}
	}
}

func OnlyofficeFileType(documentType OnlyofficeDocumentType, ext string) string {
	switch documentType {
	case "word":
		if ext == "doc" {
			return "doc"
		}
		return "docx"
	case "cell":
		if ext == "xls" {
			return "xls"
		}
		return "xlsx"
	case "slide":
		if ext == "ppt" {
			return "ppt"
		}
		return "pptx"
	default:
		return "pdf"
	}
}

func MimeForExtension(ext string) string {
	m := map[string]string{
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"doc":  "application/msword",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"xls":  "application/vnd.ms-excel",
		"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"ppt":  "application/vnd.ms-powerpoint",
		"pdf":  "application/pdf",
		"png":  "image/png",
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"ofd":  "application/ofd",
	}
	if v, ok := m[ext]; ok {
		return v
	}
	return "application/octet-stream"
}
