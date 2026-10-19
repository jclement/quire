import { describe, expect, test } from "bun:test";
import type { DocMeta } from "../api/types.ts";
import { closedRecently, compareForBoard, rankWrites } from "./projects.ts";

function project(
  path: string,
  rank: number | null,
  extra: Partial<DocMeta["project"] & object> = {},
  mtime = "2026-10-01T10:00:00Z",
): DocMeta {
  return {
    path,
    type: "project",
    title: path,
    mtime,
    sha256: "",
    tags: [],
    area: "",
    area_from: "",
    project: {
      status: "active",
      rank,
      closed: "",
      due: "",
      company: "",
      ...extra,
    },
  };
}

describe("rankWrites", () => {
  test("a move between ranked neighbours writes only the moved project", () => {
    const column = [project("a", 1000), project("m", 5000), project("b", 2000)];
    expect([...rankWrites(column, "m")]).toEqual([["m", 1500]]);
  });

  test("the ends extend past the neighbour", () => {
    expect([
      ...rankWrites([project("m", null), project("a", 1000)], "m"),
    ]).toEqual([["m", 0]]);
    expect([
      ...rankWrites([project("a", 1000), project("m", null)], "m"),
    ]).toEqual([["m", 2000]]);
    expect([...rankWrites([project("m", null)], "m")]).toEqual([["m", 1000]]);
  });

  test("an unranked column is numbered afresh, once", () => {
    const column = [project("a", null), project("m", null), project("b", null)];
    expect([...rankWrites(column, "m")]).toEqual([
      ["a", 1000],
      ["m", 2000],
      ["b", 3000],
    ]);
  });

  test("no room between neighbours renumbers only what changes", () => {
    const column = [project("a", 1000), project("m", 7), project("b", 1000)];
    expect([...rankWrites(column, "m")]).toEqual([
      ["m", 2000],
      ["b", 3000],
    ]);
  });
});

test("board order: ranked first, then newest", () => {
  const docs = [
    project("old", null, {}, "2026-01-01T00:00:00Z"),
    project("second", 2000),
    project("new", null, {}, "2026-10-01T00:00:00Z"),
    project("first", 1000),
  ];
  expect(docs.sort(compareForBoard).map((d) => d.path)).toEqual([
    "first",
    "second",
    "new",
    "old",
  ]);
});

test("a closed project folds away after the window, by its stamp or else its mtime", () => {
  const now = new Date("2026-10-18T12:00:00");
  expect(
    closedRecently(
      project("a", null, { status: "complete", closed: "2026-10-10" }),
      now,
    ),
  ).toBe(true);
  expect(
    closedRecently(
      project("a", null, { status: "complete", closed: "2026-09-01" }),
      now,
    ),
  ).toBe(false);
  expect(
    closedRecently(
      project("a", null, { status: "complete" }, "2026-10-17T00:00:00Z"),
      now,
    ),
  ).toBe(true);
  expect(
    closedRecently(
      project("a", null, { status: "complete" }, "2026-08-17T00:00:00Z"),
      now,
    ),
  ).toBe(false);
});
