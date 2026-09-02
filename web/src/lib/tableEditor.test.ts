import { describe, expect, test } from "bun:test";
import {
  addColumn,
  addRow,
  cycleAlign,
  removeColumn,
  removeRow,
  setCell,
} from "./tableEditor.ts";
import type { TableModel } from "./tables.ts";

const base: TableModel = {
  aligns: ["none", "left"],
  rows: [
    ["h1", "h2"],
    ["a", "b"],
  ],
};

describe("grid edits", () => {
  test("rows and columns come and go, header and last column stay", () => {
    const grown = addRow(addColumn(base, 1), 2);
    expect(grown.aligns).toEqual(["none", "none", "left"]);
    expect(grown.rows).toEqual([
      ["h1", "", "h2"],
      ["a", "", "b"],
      ["", "", ""],
    ]);
    expect(removeRow(base, 0)).toBe(base);
    expect(removeRow(base, 1).rows).toEqual([["h1", "h2"]]);
    const one = removeColumn(base, 0);
    expect(one.aligns).toEqual(["left"]);
    expect(removeColumn(one, 0)).toBe(one);
  });

  test("setCell and cycleAlign are immutable", () => {
    const next = setCell(base, 1, 0, "z");
    expect(next.rows[1]![0]).toBe("z");
    expect(base.rows[1]![0]).toBe("a");
    let m = base;
    const seen: string[] = [];
    for (let i = 0; i < 5; i++) {
      m = cycleAlign(m, 0);
      seen.push(m.aligns[0]!);
    }
    expect(seen).toEqual(["left", "center", "right", "none", "left"]);
  });
});
