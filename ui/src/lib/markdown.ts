// A small markdown renderer for the documentation files. It makes HTML for {@html}, so it escapes every
// character of the text first: a file cannot add a tag. It knows what the files use:
// paragraphs, lists, tables, code blocks, headings from level 4, and in a line: `code`, **bold**, *italic*, and
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

/** cells splits a row of a table. A | in `code` or after a backslash is text. */
function cells(row: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inCode = false;
  for (let i = 0; i < row.length; i++) {
    const c = row[i];
    if (c === "\\" && row[i + 1] === "|") {
      cur += "|";
      i++;
    } else if (c === "`") {
      inCode = !inCode;
      cur += c;
    } else if (c === "|" && !inCode) {
      out.push(cur.trim());
      cur = "";
    } else cur += c;
  }
  out.push(cur.trim());
  // the | at the start and at the end of a row make an empty part: not a cell
  if (row.trim().startsWith("|")) out.shift();
  if (row.trim().endsWith("|") && !row.trim().endsWith("\\|")) out.pop();
  return out;
}

const isDashes = (line: string | undefined) => !!line && /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/.test(line) && line.includes("|");

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
    // a table: a row with a |, and under it a line of dashes
    if (line.includes("|") && isDashes(lines[i + 1])) {
      endPara();
      endList();
      const row = (tag: "th" | "td", cs: string[]) => `<tr>${cs.map((c) => `<${tag}>${inline(c)}</${tag}>`).join("")}</tr>`;
      const head = cells(line);
      const body: string[] = [];
      for (i += 2; i < lines.length && lines[i].trim() && lines[i].includes("|"); i++) body.push(row("td", cells(lines[i])));
      i--; // the line after the table is read by the loop
      out.push(`<table><thead>${row("th", head)}</thead><tbody>${body.join("")}</tbody></table>`);
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
