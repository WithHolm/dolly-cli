import assert from "node:assert/strict";
import { test } from "node:test";
import { render } from "./markdown.ts";

test("paragraphs, and a line break inside a paragraph is a space", () => {
  assert.equal(render("One\ntwo.\n\nThree."), "<p>One two.</p>\n<p>Three.</p>");
});

test("code in a line is not changed, and its text is escaped", () => {
  assert.equal(render("Use `$filter=a<b` and `**not bold**`."), "<p>Use <code>$filter=a&lt;b</code> and <code>**not bold**</code>.</p>");
});

test("bold, italic, and links", () => {
  assert.equal(
    render("A **bold** and *italic* word, snake_case, 2*3*4, and [Learn](https://learn.microsoft.com/x?a=1&b=2)."),
    '<p>A <strong>bold</strong> and <em>italic</em> word, snake_case, 2*3*4, and <a href="https://learn.microsoft.com/x?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">Learn</a>.</p>',
  );
  assert.equal(render("[here](#/graph/users)"), '<p><a href="#/graph/users">here</a></p>');
});

test("a file cannot add a tag, a script, or a link that runs code", () => {
  assert.equal(render('<script>alert(1)</script> <img src=x onerror="y">'), "<p>&lt;script&gt;alert(1)&lt;/script&gt; &lt;img src=x onerror=&quot;y&quot;&gt;</p>");
  assert.equal(render("[x](javascript:alert(1))"), "<p>[x](javascript:alert(1))</p>");
  assert.equal(render("[x](data:text/html,hi)"), "<p>[x](data:text/html,hi)</p>");
  assert.equal(render('[a"b](https://e.com/"onmouseover="x)').includes('onmouseover="'), false);
  assert.ok(!render("`</code><script>`").includes("<script>"));
});

test("lists, with an item that goes on in the next line", () => {
  assert.equal(render("- one\n- two\n  more\n\n1. a\n2. b"), "<ul><li>one</li><li>two more</li></ul>\n<ol><li>a</li><li>b</li></ol>");
});

test("a code block keeps its lines, and a title inside it is code", () => {
  assert.equal(render("```\n#### not a heading\n<b>\n```"), "<pre><code>#### not a heading\n&lt;b&gt;</code></pre>");
});

test("headings from level 4", () => {
  assert.equal(render("#### Detail\ntext"), "<h4>Detail</h4>\n<p>text</p>");
});

test("an empty text is empty", () => {
  assert.equal(render(""), "");
  assert.equal(render("\n\n"), "");
});
