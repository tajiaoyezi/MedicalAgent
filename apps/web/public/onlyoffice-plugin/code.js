(function (window) {
  const ALLOWED_ORIGINS = new Set([
    "http://localhost:5173",
    "http://127.0.0.1:5173",
    "http://host.docker.internal:5173",
    "http://localhost:8080",
    "http://127.0.0.1:8080",
    "http://host.docker.internal:8080",
  ]);

  let revision = "";

  function reply(requestId, payload) {
    if (!window.parent) return;
    window.parent.postMessage(
      { channel: "medoffice-bridge", requestId, ...payload },
      "*",
    );
  }

  function readSelection() {
    return new Promise((resolve) => {
      window.Asc.plugin.executeMethod("GetSelectedText", [], (text) => {
        resolve(text || "");
      });
    });
  }

  function readFullText() {
    return new Promise((resolve) => {
      window.Asc.plugin.callCommand(
        function () {
          var oDocument = Api.GetDocument();
          return oDocument.GetText();
        },
        false,
        false,
        function (text) {
          resolve(text || "");
        },
      );
    });
  }

  function readCurrentParagraph() {
    return new Promise((resolve) => {
      window.Asc.plugin.callCommand(
        function () {
          var oDocument = Api.GetDocument();
          var oRange = oDocument.GetRangeBySelect();
          if (!oRange) return { text: "", paragraphIndex: 0 };
          return {
            text: oRange.GetText(),
            paragraphIndex: oRange.GetStartPos ? oRange.GetStartPos() : 0,
          };
        },
        false,
        false,
        function (result) {
          resolve(result || { text: "", paragraphIndex: 0 });
        },
      );
    });
  }

  async function handleMethod(method, params) {
    const docKey = window.Asc.plugin.info?.documentId || "";
    switch (method) {
      case "getCurrentDocument":
        return { documentId: docKey };
      case "getDocumentId":
        return { documentId: docKey };
      case "getDocumentTitle":
        return { title: window.Asc.plugin.info?.documentTitle || "" };
      case "getDocumentType":
        return { type: window.Asc.plugin.info?.documentType || "word" };
      case "getSelectedText": {
        const text = await readSelection();
        return {
          text,
          range: params?.range || null,
          paragraphIndex: params?.paragraphIndex ?? 0,
          page: params?.page ?? 1,
        };
      }
      case "getFullText": {
        const text = await readFullText();
        return { text, outline: [] };
      }
      case "getCurrentParagraph": {
        const para = await readCurrentParagraph();
        return {
          text: para.text || "",
          paragraphIndex: para.paragraphIndex ?? 0,
        };
      }
      case "getDocumentOutline":
        return { outline: [] };
      case "getCurrentPage":
        return { page: 1 };
      case "getComments":
        return { comments: [] };
      case "getReferences":
        return { references: [] };
      case "replaceSelection":
        return new Promise((resolve) => {
          window.Asc.plugin.executeMethod(
            "PasteText",
            [params?.text || ""],
            () =>
              resolve({
                applied: true,
                previousSelection: params?.originalText,
              }),
          );
        });
      case "insertText":
        return new Promise((resolve) => {
          window.Asc.plugin.executeMethod(
            "PasteText",
            [params?.text || ""],
            () => resolve({ inserted: true }),
          );
        });
      case "insertComment":
        return { commented: true, range: params?.range };
      case "insertCitation":
        return { inserted: true, citation: params?.citation };
      case "appendSection":
        return { appended: true };
      case "applyStyle":
        return { styled: true };
      case "createNewDocument":
        return { created: true, templateId: params?.templateId };
      case "createPresentation":
        return { created: true, slideOutline: params?.slideOutline };
      case "saveDocument":
        return new Promise((resolve, reject) => {
          window.Asc.plugin.callCommand(
            function () {
              Api.Save();
              return true;
            },
            false,
            false,
            function (ok) {
              if (ok) resolve({ saveTriggered: true });
              else reject(new Error("保存命令未执行"));
            },
          );
        });
      default:
        throw new Error("未知方法: " + method);
    }
  }

  window.addEventListener("message", async (event) => {
    if (!ALLOWED_ORIGINS.has(event.origin)) return;
    const data = event.data;
    if (!data || data.channel !== "medoffice-bridge-host") return;

    const { requestId, method, params, revision: rev } = data;
    if (rev) revision = rev;

    try {
      const result = await handleMethod(method, params || {});
      reply(requestId, {
        ok: true,
        data: result,
        docKey: window.Asc.plugin.info?.documentId,
        revision,
      });
    } catch (err) {
      reply(requestId, {
        ok: false,
        error: err instanceof Error ? err.message : String(err),
      });
    }
  });

  window.Asc.plugin.init = function () {
    window.parent.postMessage(
      { channel: "medoffice-bridge-ready", ready: true },
      "*",
    );
  };
})(window);
