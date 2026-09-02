import { describe, expect, test } from "bun:test";
import { parseTaskLine, serializeTaskLine } from "./taskLine.ts";

describe("task line grammar", () => {
  test("parses every marker regardless of order", () => {
    const t = parseTaskLine(
      "  * [x] Ship it 📅 2026-09-10 ⏫ 🔁 every  week ⏳ 🛫 2026-09-08 ✅ 2026-09-02",
    )!;
    expect(t).toEqual({
      prefix: "  * ",
      done: true,
      text: "Ship it",
      due: "2026-09-10",
      defer: "2026-09-08",
      completedOn: "2026-09-02",
      waiting: true,
      priority: 1,
      recur: "every week",
    });
    expect(serializeTaskLine(t)).toBe(
      "  * [x] Ship it ⏫ 📅 2026-09-10 🛫 2026-09-08 ⏳ 🔁 every week ✅ 2026-09-02",
    );
  });

  test("a plain task round-trips and a non-task is null", () => {
    const t = parseTaskLine("- [ ] call #ops about [[Sarah]]")!;
    expect(t.text).toBe("call #ops about [[Sarah]]");
    expect(serializeTaskLine(t)).toBe("- [ ] call #ops about [[Sarah]]");
    expect(parseTaskLine("- just a bullet")).toBeNull();
    expect(parseTaskLine("plain")).toBeNull();
  });

  test("a marker without a date is dropped, not kept as text", () => {
    const t = parseTaskLine("- [ ] thing 📅")!;
    expect(t.due).toBe("");
    expect(serializeTaskLine(t)).toBe("- [ ] thing");
  });
});
