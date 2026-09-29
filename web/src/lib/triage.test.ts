import { describe, expect, test } from "bun:test";
import type { Task } from "../api/types.ts";
import { TRIAGE_ACTIONS, triageStep, withTag } from "./triage.ts";

const task = { id: "a", text: "Call the bank" } as Task;
// 2026-09-28 is a Monday: "next Monday" must mean the one after, not today.
const monday = "2026-09-28";

describe("triage keys", () => {
  test("date keys resolve against today", () => {
    expect(triageStep("t", task, monday)).toMatchObject({
      edit: { due: "2026-09-28" },
    });
    expect(triageStep("m", task, monday)).toMatchObject({
      edit: { due: "2026-09-29" },
    });
    expect(triageStep("w", task, monday)).toMatchObject({
      edit: { due: "2026-10-05" },
    });
    expect(triageStep("w", task, "2026-10-02")).toMatchObject({
      edit: { due: "2026-10-05" },
    });
  });

  test("someday tags once", () => {
    expect(triageStep("s", task, monday)).toMatchObject({
      edit: { text: "Call the bank #someday" },
    });
    expect(withTag("Call the bank #Someday", "someday")).toBe(
      "Call the bank #Someday",
    );
    // A longer tag that merely starts with the word is a different tag.
    expect(withTag("x #somedays", "someday")).toBe("x #somedays #someday");
  });

  test("pickers and the note dialog ask first", () => {
    expect(triageStep("d", task, monday)).toEqual({
      kind: "pick",
      field: "due",
    });
    expect(triageStep("f", task, monday)).toEqual({
      kind: "pick",
      field: "defer",
    });
    expect(triageStep("n", task, monday)).toEqual({ kind: "note" });
    expect(triageStep("p", task, monday)).toMatchObject({
      edit: { waiting: true },
    });
  });

  test("every key is distinct", () => {
    const keys = TRIAGE_ACTIONS.map((action) => action.key);
    expect(new Set(keys).size).toBe(keys.length);
  });
});
