import type { PermissionLevel } from "./document-permissions.js";

export type BridgeMethodCategory = "read" | "write" | "write_comment" | "panel";

const READ_METHODS = new Set([
  "getCurrentDocument",
  "getDocumentId",
  "getDocumentTitle",
  "getDocumentType",
  "getFullText",
  "getSelectedText",
  "getCurrentParagraph",
  "getDocumentOutline",
  "getCurrentPage",
  "getComments",
  "getReferences",
]);

const WRITE_COMMENT_METHODS = new Set(["insertComment"]);

const WRITE_METHODS = new Set([
  "replaceSelection",
  "insertText",
  "appendSection",
  "insertCitation",
  "applyStyle",
  "createNewDocument",
  "createPresentation",
  "saveDocument",
]);

const PANEL_METHODS = new Set([
  "openAIPanel",
  "closeAIPanel",
  "runAIPanelSkill",
  "streamContentToEditor",
]);

export function categorizeBridgeMethod(method: string): BridgeMethodCategory | null {
  if (READ_METHODS.has(method)) return "read";
  if (WRITE_COMMENT_METHODS.has(method)) return "write_comment";
  if (WRITE_METHODS.has(method)) return "write";
  if (PANEL_METHODS.has(method)) return "panel";
  return null;
}

export function minPermissionForBridge(
  category: BridgeMethodCategory,
): PermissionLevel {
  if (category === "read" || category === "panel") return "view";
  if (category === "write_comment") return "comment";
  return "edit";
}

export function isBridgeMethodAllowed(
  category: BridgeMethodCategory,
  level: PermissionLevel,
): boolean {
  const min = minPermissionForBridge(category);
  const order: Record<PermissionLevel, number> = {
    none: 0,
    view: 1,
    comment: 2,
    edit: 3,
    manage: 4,
    owner: 5,
  };
  return order[level] >= order[min];
}

export function isTextExportMethod(method: string): boolean {
  return (
    method === "getFullText" ||
    method === "getSelectedText" ||
    method === "getCurrentParagraph"
  );
}

export function requiresRevisionCheck(method: string): boolean {
  return WRITE_METHODS.has(method);
}
