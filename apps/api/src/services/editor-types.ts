export type EditorRoute =
  | "onlyoffice"
  | "preview-pdf"
  | "preview-image"
  | "preview-ofd"
  | "unsupported";

export type OnlyofficeDocumentType = "word" | "cell" | "slide" | "pdf";

const WORD_EXT = new Set(["docx", "doc"]);
const CELL_EXT = new Set(["xlsx", "xls"]);
const SLIDE_EXT = new Set(["pptx", "ppt"]);
const PDF_EXT = new Set(["pdf"]);
const IMAGE_EXT = new Set(["png", "jpg", "jpeg"]);
const OFD_EXT = new Set(["ofd"]);

export function extensionOf(filename: string): string {
  const dot = filename.lastIndexOf(".");
  if (dot < 0) return "";
  return filename.slice(dot + 1).toLowerCase();
}

export function resolveEditorRoute(
  filename: string,
): {
  route: EditorRoute;
  documentType?: OnlyofficeDocumentType;
  fileType: string;
} {
  const ext = extensionOf(filename);
  if (WORD_EXT.has(ext)) {
    return { route: "onlyoffice", documentType: "word", fileType: ext };
  }
  if (CELL_EXT.has(ext)) {
    return { route: "onlyoffice", documentType: "cell", fileType: ext };
  }
  if (SLIDE_EXT.has(ext)) {
    return { route: "onlyoffice", documentType: "slide", fileType: ext };
  }
  if (PDF_EXT.has(ext)) {
    return { route: "preview-pdf", documentType: "pdf", fileType: ext };
  }
  if (IMAGE_EXT.has(ext)) {
    return { route: "preview-image", fileType: ext };
  }
  if (OFD_EXT.has(ext)) {
    return { route: "preview-ofd", fileType: ext };
  }
  return { route: "unsupported", fileType: ext };
}

export function onlyofficeFileType(
  documentType: OnlyofficeDocumentType,
  ext: string,
): string {
  if (documentType === "word") {
    return ext === "doc" ? "doc" : "docx";
  }
  if (documentType === "cell") {
    return ext === "xls" ? "xls" : "xlsx";
  }
  if (documentType === "slide") {
    return ext === "ppt" ? "ppt" : "pptx";
  }
  return "pdf";
}

export function mimeForExtension(ext: string): string {
  const map: Record<string, string> = {
    docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    doc: "application/msword",
    xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    xls: "application/vnd.ms-excel",
    pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
    ppt: "application/vnd.ms-powerpoint",
    pdf: "application/pdf",
    png: "image/png",
    jpg: "image/jpeg",
    jpeg: "image/jpeg",
    ofd: "application/ofd",
  };
  return map[ext] ?? "application/octet-stream";
}
