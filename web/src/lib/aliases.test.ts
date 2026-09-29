// The Add alias button's failure message: a 409 means the page changed
// under the write, which is worth saying in words the row's user knows.
import { describe, expect, test } from "bun:test";
import { ApiError } from "../api/client.ts";
import { addAliasErrorMessage } from "./aliases.ts";

describe("addAliasErrorMessage", () => {
  test("a conflict says which page changed and what to do", () => {
    const conflict = new ApiError(
      "CONFLICT",
      "the file changed on disk — reload and reapply your edit",
      409,
    );
    expect(addAliasErrorMessage(conflict, "Frances", "Frances Bagley")).toBe(
      "Frances Bagley changed while “Frances” was being added — nothing was written. Try again.",
    );
  });
  test("anything else passes the server's message through", () => {
    const invalid = new ApiError(
      "VALIDATION_ERROR",
      'alias "a|b" cannot contain [ ] | # ^ or a line break',
      400,
    );
    expect(addAliasErrorMessage(invalid, "a|b", "X")).toBe(
      'alias "a|b" cannot contain [ ] | # ^ or a line break',
    );
  });
});
