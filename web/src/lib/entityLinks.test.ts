// Tests for the properties strip's link reading. The risk here is the shapes
// frontmatter actually arrives in — a scalar where a list was expected, an
// aliased wikilink, a value a human typed without brackets — since guessing
// wrong means a chip that can't be removed because its target doesn't match
// what the server stored.
import { describe, expect, test } from "bun:test";
import { linkKeysFor, linkTargets, resolvedTargets } from "./entityLinks.ts";

describe("linkKeysFor", () => {
  test("meetings offer people, project and company in that order", () => {
    expect(linkKeysFor("meeting").map((entry) => entry.key)).toEqual([
      "people",
      "project",
      "company",
    ]);
  });

  test("company, project and people search their own document type", () => {
    const byKey = new Map(
      linkKeysFor("meeting").map((entry) => [entry.key, entry]),
    );
    expect(byKey.get("company")?.type).toBe("company");
    expect(byKey.get("people")?.type).toBe("person");
    expect(byKey.get("project")?.type).toBe("project");
  });

  test("company and project are scalars, people is a list", () => {
    const byKey = new Map(
      linkKeysFor("meeting").map((entry) => [entry.key, entry]),
    );
    expect(byKey.get("company")?.singular).toBe(true);
    expect(byKey.get("project")?.singular).toBe(true);
    expect(byKey.get("people")?.singular).toBe(false);
  });

  test("types with nothing to point at offer nothing", () => {
    expect(linkKeysFor("company")).toEqual([]);
    expect(linkKeysFor("note")).toEqual([]);
    expect(linkKeysFor("daily")).toEqual([]);
  });
});

describe("linkTargets", () => {
  test("reads a scalar wikilink and a list alike", () => {
    expect(linkTargets("[[Acme]]")).toEqual(["Acme"]);
    expect(linkTargets(["[[Sarah Chen]]", "[[Dan Roe]]"])).toEqual([
      "Sarah Chen",
      "Dan Roe",
    ]);
  });

  test("an aliased link resolves by its target, not its display text", () => {
    expect(linkTargets("[[Sarah Chen|Sarah]]")).toEqual(["Sarah Chen"]);
  });

  test("a hand-typed value without brackets is still a target", () => {
    expect(linkTargets("Acme")).toEqual(["Acme"]);
    expect(linkTargets(["Acme", "[[Globex]]"])).toEqual(["Acme", "Globex"]);
  });

  test("missing keys and non-string values yield nothing", () => {
    expect(linkTargets(undefined)).toEqual([]);
    expect(linkTargets(null)).toEqual([]);
    expect(linkTargets("   ")).toEqual([]);
    expect(linkTargets([42, { name: "Acme" }])).toEqual([]);
  });
});

describe("resolvedTargets", () => {
  test("maps link text to its path, case-insensitively, skipping danglers", () => {
    const resolved = resolvedTargets([
      { target: "companies/acme.md", raw: "Acme", display: "Acme" },
      { target: null, raw: "Ghost Co", display: "Ghost Co" },
    ]);
    expect(resolved.get("acme")).toBe("companies/acme.md");
    expect(resolved.has("ghost co")).toBe(false);
  });
});
