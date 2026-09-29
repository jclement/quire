// The editor's work-item highlighting must agree with the renderer about
// what a work item is — same spellings, same boundaries.
import { describe, expect, test } from "bun:test";
import { GFM, parser } from "@lezer/markdown";
import { workItemMark } from "./workItemMark.ts";

const markdown = parser.configure([GFM, workItemMark]);

function workItems(text: string): string[] {
  const out: string[] = [];
  markdown.parse(text).iterate({
    enter(node) {
      if (node.name === "WorkItem") out.push(text.slice(node.from, node.to));
    },
  });
  return out;
}

describe("workItemMark", () => {
  test("marks the spellings the renderer links", () => {
    expect(workItems("- [ ] #2433 Auto-update (AB#2440)\n")).toEqual([
      "#2433",
      "AB#2440",
    ]);
  });
  test("leaves tags, anchors and glued numbers alone", () => {
    expect(workItems("#ops page#12 ZAB#3 #12abc #1-2 `#44`\n")).toEqual([]);
  });
});
