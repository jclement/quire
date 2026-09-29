// Tests for the remark plugin's work-item transform, at the mdast level the
// renderer consumes: which text becomes a work-item link, and which stays
// what it was — a tag, code, an existing link's label.
import { describe, expect, test } from "bun:test";
import type { Link, Root, RootContent } from "mdast";
import remarkGfm from "remark-gfm";
import remarkParse from "remark-parse";
import { unified } from "unified";
import { visit } from "unist-util-visit";
import { remarkQuire, TAG_HREF_PREFIX } from "./remarkQuire.ts";
import { WORK_ITEM_HREF_PREFIX } from "./workItems.ts";

function links(markdown: string): { url: string; label: string }[] {
  const processor = unified().use(remarkParse).use(remarkGfm).use(remarkQuire);
  const tree = processor.runSync(processor.parse(markdown)) as Root;
  const out: { url: string; label: string }[] = [];
  visit(tree, "link", (node: Link) => {
    const label = node.children
      .map((child: RootContent) => ("value" in child ? child.value : ""))
      .join("");
    out.push({ url: node.url, label });
  });
  return out;
}

describe("remarkQuire work items", () => {
  test("a task's leading number and an AB# reference become work items", () => {
    expect(
      links("- [ ] #2433 Auto-update of environments (see AB#2440) #ops\n"),
    ).toEqual([
      { url: WORK_ITEM_HREF_PREFIX + "2433", label: "#2433" },
      { url: WORK_ITEM_HREF_PREFIX + "2440", label: "AB#2440" },
      { url: TAG_HREF_PREFIX + "ops", label: "#ops" },
    ]);
  });

  test("inside code or an existing link, a number is left alone", () => {
    expect(links("`#2433` and [ticket #2433](https://example.com)\n")).toEqual([
      { url: "https://example.com", label: "ticket #2433" },
    ]);
  });

  test("a number or tag nested deeper inside a link is left alone", () => {
    // An <a> inside an <a> is invalid HTML and the inner one hijacks the click.
    expect(links("[fixed **#12** and *#ops*](https://example.com)\n")).toEqual([
      { url: "https://example.com", label: "fixed  and " },
    ]);
  });

  test("a work item inside a highlight is still one", () => {
    expect(links("==ship #12 today==\n")).toEqual([
      { url: WORK_ITEM_HREF_PREFIX + "12", label: "#12" },
    ]);
  });
});
