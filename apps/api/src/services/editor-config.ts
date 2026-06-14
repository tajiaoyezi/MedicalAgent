import { config } from "../config.js";
import {
  canComment,
  canCopy,
  canEdit,
  type PermissionLevel,
} from "./document-permissions.js";
import {
  mimeForExtension,
  onlyofficeFileType,
  type OnlyofficeDocumentType,
} from "./editor-types.js";
import type { EditorSession } from "./editor-sessions.js";
import { wrapOnlyofficeConfig } from "./onlyoffice-jwt.js";

export function buildDocumentKey(documentId: string, versionId: string): string {
  return `${documentId}_${versionId}`;
}

export function buildEditorConfig(input: {
  session: EditorSession;
  filename: string;
  documentType: OnlyofficeDocumentType;
  permission: PermissionLevel;
  user: { userId: string; displayName: string };
}): Record<string, unknown> {
  const { session, filename, documentType, permission, user } = input;
  const ext = filename.includes(".")
    ? filename.slice(filename.lastIndexOf(".") + 1).toLowerCase()
    : "docx";
  const editable = canEdit(permission);
  const commentable = canComment(permission);
  const copyable = canCopy(permission);

  const downloadUrl = `${config.onlyoffice.apiPublicUrl}/api/editor/download/${session.openToken}`;
  const needsCallback = editable || (commentable && !editable);
  const callbackUrl = needsCallback
    ? `${config.onlyoffice.apiPublicUrl}/api/editor/callback?token=${session.callbackToken}`
    : undefined;

  const coreConfig = {
    documentType,
    document: {
      fileType: onlyofficeFileType(documentType, ext),
      key: session.documentKey,
      title: filename,
      url: downloadUrl,
      permissions: {
        edit: editable,
        comment: commentable && !editable,
        copy: copyable,
        download: true,
        print: true,
      },
    },
    editorConfig: {
      mode: editable || (commentable && !editable) ? "edit" : "view",
      lang: "zh-CN",
      callbackUrl,
      user: {
        id: user.userId,
        name: user.displayName,
      },
      customization: {
        forcesave: true,
        plugins: true,
      },
      plugins: {
        autostart: ["asc.{medoffice-bridge}"],
        pluginsData: [`${config.onlyoffice.pluginUrl}config.json`],
      },
    },
  };

  const wrapped = wrapOnlyofficeConfig(coreConfig);
  return {
    ...wrapped,
    bridgeToken: session.bridgeToken,
    revision: session.revision,
    mimeType: mimeForExtension(ext),
  };
}
