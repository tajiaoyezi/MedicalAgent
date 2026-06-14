// c03｜chunk 切分（design D7）：文本型按段落/章节切分；视觉解析结构化输出按页码/段落切分。
import type { VisualParseResult } from "./visual-parse.js";

export interface TextSegment {
  text: string;
  page: number | null;
  paragraphIndex: number;
  section: string | null;
}

const DEFAULT_MAX_CHARS = 800;

/** 文本型：按空行分段，连续小段合并到 ~maxChars，标题（# / 数字编号）作为 section。 */
export function chunkPlainText(
  text: string,
  maxChars = DEFAULT_MAX_CHARS,
): TextSegment[] {
  const paragraphs = text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean);

  const segments: TextSegment[] = [];
  let buffer = "";
  let section: string | null = null;
  let index = 0;

  const flush = () => {
    if (buffer.trim()) {
      segments.push({ text: buffer.trim(), page: null, paragraphIndex: index++, section });
      buffer = "";
    }
  };

  for (const para of paragraphs) {
    const headingMatch = /^(#{1,6}\s+.+|第[一二三四五六七八九十\d]+[章节].*)$/.exec(para);
    if (headingMatch) {
      flush();
      section = para.replace(/^#{1,6}\s+/, "").slice(0, 120);
      buffer = para;
      flush();
      continue;
    }
    if (buffer.length + para.length > maxChars) flush();
    buffer = buffer ? `${buffer}\n${para}` : para;
  }
  flush();
  return segments;
}

/** 视觉解析输出：以页码/段落/标题层级为切分依据，保持页码与段落可溯源（引用误差 ≤ 1 页）。 */
export function chunkFromVisual(result: VisualParseResult): TextSegment[] {
  const segments: TextSegment[] = [];
  for (const page of result.pages) {
    for (const para of page.paragraphs) {
      if (!para.text.trim()) continue;
      segments.push({
        text: para.text.trim(),
        page: page.page,
        paragraphIndex: para.paragraphIndex,
        section:
          para.headingLevel !== undefined && para.headingLevel <= 2
            ? para.text.slice(0, 120)
            : null,
      });
    }
  }
  return segments;
}
