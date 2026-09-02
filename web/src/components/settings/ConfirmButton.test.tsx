// ConfirmButton guards every irreversible action in Settings — revoking a
// token, disconnecting an app, killing a share link, deleting a passkey —
// so the thing worth testing is that it never fires on a single click, and
// that it does not stay armed waiting to be hit by accident later.
import { describe, expect, mock, test } from "bun:test";
import { fireEvent, render, screen } from "@testing-library/react";
import { ConfirmButton } from "./ConfirmButton.tsx";

function renderButton(onConfirm: () => void) {
  return render(
    <ConfirmButton
      label="Revoke token claude"
      confirmLabel="Revoke?"
      onConfirm={onConfirm}
    >
      <span>icon</span>
    </ConfirmButton>,
  );
}

describe("ConfirmButton", () => {
  test("one click arms it but does not fire", () => {
    const onConfirm = mock(() => {});
    renderButton(onConfirm);

    fireEvent.click(
      screen.getByRole("button", { name: "Revoke token claude" }),
    );

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Revoke?" })).toBeDefined();
  });

  test("a second click fires exactly once", () => {
    const onConfirm = mock(() => {});
    renderButton(onConfirm);

    fireEvent.click(
      screen.getByRole("button", { name: "Revoke token claude" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Revoke?" }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    // It disarms afterwards, so the row cannot be double-fired.
    expect(
      screen.getByRole("button", { name: "Revoke token claude" }),
    ).toBeDefined();
  });

  test("looking away disarms it", () => {
    const onConfirm = mock(() => {});
    renderButton(onConfirm);

    fireEvent.click(
      screen.getByRole("button", { name: "Revoke token claude" }),
    );
    fireEvent.blur(screen.getByRole("button", { name: "Revoke?" }));

    // Back to resting, and nothing happened.
    expect(
      screen.getByRole("button", { name: "Revoke token claude" }),
    ).toBeDefined();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  test("the resting button carries the full accessible name", () => {
    // The armed label is deliberately terse ("Revoke?"), so the resting
    // state has to say what it would revoke — otherwise a screen reader
    // hears a row of identical buttons.
    renderButton(() => {});
    expect(screen.getByLabelText("Revoke token claude")).toBeDefined();
  });
});
