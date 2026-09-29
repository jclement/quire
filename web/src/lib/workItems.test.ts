// Tests for work-item references: which spellings are tickets, which are
// not (tags, anchors, prices), and URL expansion from the Settings template.
import { describe, expect, test } from "bun:test";
import { splitWorkItems, workItemUrl } from "./workItems.ts";

describe("splitWorkItems", () => {
  test("a leading numeric hashtag in a task is a work item", () => {
    expect(splitWorkItems("#2433 Auto-update of environments")).toEqual([
      { kind: "workitem", id: "2433", label: "#2433" },
      { kind: "text", text: " Auto-update of environments" },
    ]);
  });

  test("AB# is the Azure Boards spelling, kept as written", () => {
    expect(splitWorkItems("fixed in AB#2440, see (#2441).")).toEqual([
      { kind: "text", text: "fixed in " },
      { kind: "workitem", id: "2440", label: "AB#2440" },
      { kind: "text", text: ", see (" },
      { kind: "workitem", id: "2441", label: "#2441" },
      { kind: "text", text: ")." },
    ]);
  });

  test("tags, anchors and glued numbers are not work items", () => {
    for (const text of [
      "#ops and #q3",
      "page#123",
      "ZAB#123",
      "#123abc",
      "#12-3",
      "issue #",
    ]) {
      expect(splitWorkItems(text)).toEqual([{ kind: "text", text }]);
    }
  });
});

describe("workItemUrl", () => {
  const ado = "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/{id}";
  test("fills the template", () => {
    expect(workItemUrl(ado, "2433")).toBe(
      "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/2433",
    );
  });
  test("no template means no link", () => {
    expect(workItemUrl("", "2433")).toBeNull();
  });
});
