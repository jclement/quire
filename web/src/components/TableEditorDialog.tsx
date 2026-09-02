// The visual table editor: a grid of inputs over a GFM table, so cells can be
// filled in without counting pipes or remembering that `|` inside a cell
// must be written `\|`. Opened by requestTableEdit() from either read mode's
// rendered tables or the editor's table panel; on save it hands back
// reformatted markdown and the caller decides where it goes. One instance
// lives in the root layout.
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Minus,
  Plus,
  X,
} from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import {
  addColumn,
  addRow,
  cycleAlign,
  onTableEditRequest,
  removeColumn,
  removeRow,
  setCell,
  type TableEditRequest,
} from "../lib/tableEditor.ts";
import {
  parseTable,
  serializeTable,
  type Alignment,
  type TableModel,
} from "../lib/tables.ts";
import { Modal } from "./Modal.tsx";

const ALIGN_LABEL: Record<Alignment, string> = {
  none: "default",
  left: "left",
  center: "center",
  right: "right",
};

function AlignIcon({ align }: { align: Alignment }) {
  const className = "size-3.5";
  if (align === "center")
    return <AlignCenter className={className} aria-hidden="true" />;
  if (align === "right")
    return <AlignRight className={className} aria-hidden="true" />;
  return (
    <AlignLeft
      className={align === "none" ? `${className} opacity-40` : className}
      aria-hidden="true"
    />
  );
}

export function TableEditorDialog() {
  const [request, setRequest] = useState<TableEditRequest | null>(null);
  const [model, setModel] = useState<TableModel | null>(null);
  const firstCell = useRef<HTMLInputElement>(null);

  useEffect(
    () =>
      onTableEditRequest((next) => {
        const parsed = parseTable(next.block);
        if (!parsed) return;
        setRequest(next);
        setModel(parsed);
      }),
    [],
  );

  useEffect(() => {
    if (model) firstCell.current?.focus();
    // Only on open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [request]);

  const close = () => {
    setRequest(null);
    setModel(null);
  };

  const save = () => {
    if (!request || !model) return;
    request.apply(serializeTable(model));
    close();
  };

  // Enter moves down a row (adding one at the bottom) so a column can be
  // typed top to bottom; Tab already walks across cells.
  const onCellKey = (
    event: KeyboardEvent<HTMLInputElement>,
    row: number,
    column: number,
  ) => {
    if (!model) return;
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      save();
      return;
    }
    if (event.key !== "Enter") return;
    event.preventDefault();
    const next =
      row + 1 >= model.rows.length ? addRow(model, model.rows.length) : model;
    if (next !== model) setModel(next);
    requestAnimationFrame(() => {
      const input = document.querySelector<HTMLInputElement>(
        `[data-table-cell="${row + 1}:${column}"]`,
      );
      input?.focus();
    });
  };

  const open = request !== null && model !== null;

  return (
    <Modal open={open} onClose={close} variant="help" label="Edit table">
      {model ? (
        <div className="flex max-h-[90vh] flex-col md:max-h-[80vh]">
          <div className="flex items-center gap-3 border-b border-border px-4 py-2">
            <h2 className="text-sm font-semibold text-heading">Edit table</h2>
            <p className="text-xs text-muted">
              Pipes and line breaks in cells are escaped for you. Enter moves
              down, Tab moves across.
            </p>
            <button
              type="button"
              onClick={close}
              className="ml-auto rounded p-1 text-muted hover:bg-hover hover:text-heading"
              aria-label="Close"
            >
              <X className="size-4" aria-hidden="true" />
            </button>
          </div>

          <div className="min-h-0 flex-1 overflow-auto p-3">
            <table className="border-separate border-spacing-0 text-sm">
              <thead>
                <tr>
                  {model.aligns.map((align, column) => (
                    <th key={column} className="p-0.5 align-bottom">
                      <div className="flex items-center gap-0.5">
                        <button
                          type="button"
                          onClick={() => setModel(cycleAlign(model, column))}
                          className="flex h-6 items-center gap-1 rounded border border-border px-1.5 text-[11px] text-muted hover:bg-hover hover:text-heading"
                          title="Column alignment"
                          aria-label={`Column ${column + 1} alignment: ${ALIGN_LABEL[align]}`}
                        >
                          <AlignIcon align={align} />
                          {ALIGN_LABEL[align]}
                        </button>
                        <button
                          type="button"
                          onClick={() => setModel(removeColumn(model, column))}
                          disabled={model.aligns.length <= 1}
                          className="rounded p-1 text-muted hover:bg-hover hover:text-danger disabled:opacity-30"
                          aria-label={`Remove column ${column + 1}`}
                        >
                          <Minus className="size-3.5" aria-hidden="true" />
                        </button>
                      </div>
                    </th>
                  ))}
                  <th className="p-0.5 align-bottom">
                    <button
                      type="button"
                      onClick={() =>
                        setModel(addColumn(model, model.aligns.length))
                      }
                      className="flex h-6 items-center gap-1 rounded border border-dashed border-border px-1.5 text-[11px] text-muted hover:bg-hover hover:text-heading"
                      aria-label="Add column"
                    >
                      <Plus className="size-3.5" aria-hidden="true" />
                      column
                    </button>
                  </th>
                </tr>
              </thead>
              <tbody>
                {model.rows.map((cells, row) => (
                  <tr key={row}>
                    {cells.map((cell, column) => (
                      <td key={column} className="p-0.5">
                        <input
                          ref={
                            row === 0 && column === 0 ? firstCell : undefined
                          }
                          data-table-cell={`${row}:${column}`}
                          value={cell}
                          onChange={(event) =>
                            setModel(
                              setCell(model, row, column, event.target.value),
                            )
                          }
                          onKeyDown={(event) => onCellKey(event, row, column)}
                          aria-label={
                            row === 0
                              ? `Header, column ${column + 1}`
                              : `Row ${row}, column ${column + 1}`
                          }
                          className={`field-bare h-8 w-full min-w-32 rounded border border-border bg-surface px-1.5 text-heading outline-none focus:border-accent ${
                            row === 0 ? "font-semibold" : ""
                          }`}
                        />
                      </td>
                    ))}
                    <td className="p-0.5">
                      {row === 0 ? null : (
                        <button
                          type="button"
                          onClick={() => setModel(removeRow(model, row))}
                          className="rounded p-1 text-muted hover:bg-hover hover:text-danger"
                          aria-label={`Remove row ${row}`}
                        >
                          <Minus className="size-3.5" aria-hidden="true" />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button
              type="button"
              onClick={() => setModel(addRow(model, model.rows.length))}
              className="mt-1 flex h-7 items-center gap-1 rounded border border-dashed border-border px-2 text-xs text-muted hover:bg-hover hover:text-heading"
            >
              <Plus className="size-3.5" aria-hidden="true" />
              Add row
            </button>
          </div>

          <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
            <button
              type="button"
              onClick={close}
              className="flex h-8 items-center rounded border border-border px-2.5 text-xs text-body hover:bg-hover"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={save}
              className="flex h-8 items-center rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90"
              title="⌘↩"
            >
              Save table
            </button>
          </div>
        </div>
      ) : null}
    </Modal>
  );
}
