// A small markdown renderer for the documentation files. It makes HTML for {@html}, so it escapes every
// character of the text first: a file cannot add a tag. It knows what the files use:
// paragraphs, lists, code blocks, headings from level 4, and in a line: `code`, **bold**, *italic*, and
// [links](https://…). A link must be http, https, or an address in this page (#…).

const escapes: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
const escape = (s: string) => s.replace(/[&<>"']/g, (c) => escapes[c]);

const safeLink = (url: string) => /^(https?:\/\/|#)/i.test(url);

/** inline renders the parts of one line of text. */
function inline(text: string): string {
  const code: string[] = [];
  // code first: nothing inside `…` is changed, and its text must not meet the rules below
  let s = escape(text).replace(/`([^`]+)`/g, (_, c: string) => `\u0000${code.push(c) - 1}\u0000`);
  s = s
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (m, label: string, url: string) => {
      const href = url.replace(/&amp;/g, "&");
      if (!safeLink(href)) return m;
      const external = /^https?:/i.test(href);
      return `<a href="${escape(href)}"${external ? ' target="_blank" rel="noopener noreferrer"' : ""}>${label}</a>`;
    })
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/(^|[^*\w])\*([^*\s][^*]*)\*(?![*\w])/g, "$1<em>$2</em>");
  return s.replace(/\u0000(\d+)\u0000/g, (_, i: string) => `<code>${code[Number(i)]}</code>`);
}

/** render turns markdown into HTML. */
export function render(md: string): string {
  const lines = md.replace(/\r\n/g, "\n").split("\n");
  const out: string[] = [];
  let para: string[] = [];
  let list: { tag: "ul" | "ol"; items: string[] } | null = null;

  const endPara = () => {
    if (para.length) out.push(`<p>${inline(para.join(" "))}</p>`);
    para = [];
  };
  const endList = () => {
    if (list) out.push(`<${list.tag}>${list.items.map((i) => `<li>${inline(i)}</li>`).join("")}</${list.tag}>`);
    list = null;
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim().startsWith("```")) {
      endPara();
      endList();
      const body: string[] = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith("```"); i++) body.push(lines[i]);
      out.push(`<pre><code>${escape(body.join("\n"))}</code></pre>`);
      continue;
    }
    const item = /^\s*(?:([-*])|(\d+)\.)\s+(.*)$/.exec(line);
    if (item) {
      endPara();
      const tag = item[1] ? "ul" : "ol";
      if (list && list.tag !== tag) endList();
      (list ??= { tag, items: [] }).items.push(item[3]);
      continue;
    }
    // a line that goes on after an item, with an indent, belongs to the item
    if (list && /^\s+\S/.test(line)) {
      list.items[list.items.length - 1] += " " + line.trim();
      continue;
    }
    const heading = /^(#{4,6})\s+(.*)$/.exec(line);
    if (heading) {
      endPara();
      endList();
      out.push(`<h${heading[1].length}>${inline(heading[2])}</h${heading[1].length}>`);
      continue;
    }
    if (!line.trim()) {
      endPara();
      endList();
      continue;
    }
    endList();
    para.push(line.trim());
  }
  endPara();
  endList();
  return out.join("\n");
}
