// The bridge between "someone wants to edit this table as a grid" and the
// one TableEditorDialog mounted in the root layout. Two callers need it:
// read mode's rendered tables (React, could use context) and the CodeMirror
// table panel (plain DOM, lazily loaded, no React tree to reach into). A
// window event serves both without threading callbacks through the editor.
import type { TableModel } from "./tables.ts";

export interface TableEditRequest {
  /** The table's markdown, exactly as it sits in the document. */
  block: string;
  /** Called with the reformatted markdown when the user saves. */
  apply: (next: string) => void;
}

const EVENT = "quire:edit-table";

/** Asks the mounted dialog to open on this table. */
export function requestTableEdit(request: TableEditRequest): void {
  window.dispatchEvent(
    new CustomEvent<TableEditRequest>(EVENT, { detail: request }),
  );
}

/** Subscribes to edit requests; returns the unsubscribe. */
export function onTableEditRequest(
  handler: (request: TableEditRequest) => void,
): () => void {
  const listener = (event: Event) =>
    handler((event as CustomEvent<TableEditRequest>).detail);
  window.addEventListener(EVENT, listener);
  return () => window.removeEventListener(EVENT, listener);
}

// ---- pure grid edits, so the dialog's state changes are testable ----

export function addRow(model: TableModel, at: number): TableModel {
  const rows = model.rows.slice();
  rows.splice(
    at,
    0,
    model.aligns.map(() => ""),
  );
  return { ...model, rows };
}

/** The header row (index 0) cannot be removed; a table needs one. */
export function removeRow(model: TableModel, at: number): TableModel {
  if (at === 0 || model.rows.length <= 1) return model;
  return { ...model, rows: model.rows.filter((_, i) => i !== at) };
}

export function addColumn(model: TableModel, at: number): TableModel {
  const aligns = model.aligns.slice();
  aligns.splice(at, 0, "none");
  return {
    aligns,
    rows: model.rows.map((r) => {
      const next = r.slice();
      next.splice(at, 0, "");
      return next;
    }),
  };
}

/** The last column stays; an empty table is not a table. */
export function removeColumn(model: TableModel, at: number): TableModel {
  if (model.aligns.length <= 1) return model;
  return {
    aligns: model.aligns.filter((_, i) => i !== at),
    rows: model.rows.map((r) => r.filter((_, i) => i !== at)),
  };
}

export function setCell(
  model: TableModel,
  row: number,
  column: number,
  value: string,
): TableModel {
  const rows = model.rows.map((r, i) =>
    i === row ? r.map((c, j) => (j === column ? value : c)) : r,
  );
  return { ...model, rows };
}

/** Cycles none → left → center → right → none. */
export function cycleAlign(model: TableModel, column: number): TableModel {
  const order = ["none", "left", "center", "right"] as const;
  const aligns = model.aligns.map((a, i) =>
    i === column ? order[(order.indexOf(a) + 1) % order.length]! : a,
  );
  return { ...model, aligns };
}
