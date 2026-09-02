// GFM table formatting: find the table under a cursor and rewrite it with
// every column padded to its widest cell, so the source reads as a grid
// instead of a ragged pipe soup.
//
// This is a pure text transform kept away from CodeMirror so it can be
// tested exhaustively; the editor wires it to a command and a "Reformat
// table" panel. Two things make it more than a split-on-pipe:
//   - a `|` inside a backtick code span or escaped as `\|` is cell content,
//     not a separator, and must survive the round trip untouched;
//   - column width is *display* width, not string length — CJK and emoji
//     occupy two cells in a monospace editor, and padding by .length leaves
//     those columns visibly crooked.

export type Alignment = "left" | "center" | "right" | "none";

export interface TableRange {
  /** Offset of the first character of the table's first line. */
  from: number;
  /** Offset just past the last character of the table's last line. */
  to: number;
}

const DELIMITER_CELL = /^\s*:?-+:?\s*$/;

/** True when a line looks like a table row: contains an unescaped pipe. */
function isRowLine(line: string): boolean {
  return splitCells(line).length > 1 || /^\s*\|.*\|\s*$/.test(line);
}

/** True when a line is a GFM delimiter row (`| --- | :-: |`). */
export function isDelimiterLine(line: string): boolean {
  const cells = splitCells(line);
  return cells.length > 0 && cells.every((c) => DELIMITER_CELL.test(c));
}

/**
 * Splits one row into raw cell strings, honouring code spans and `\|`.
 * Leading and trailing pipes are stripped; cell text is trimmed.
 */
export function splitCells(line: string): string[] {
  const cells: string[] = [];
  let current = "";
  let inCode = false;
  let codeFence = 0; // length of the opening backtick run
  let i = 0;
  const s = line.trim();
  // A leading pipe is the row's border, not an empty first cell.
  if (s.startsWith("|")) i = 1;

  while (i < s.length) {
    const ch = s[i]!;
    if (ch === "`") {
      // Count the run so `` a|b `` style spans close on a matching run.
      let run = 0;
      while (s[i + run] === "`") run++;
      if (!inCode) {
        inCode = true;
        codeFence = run;
      } else if (run === codeFence) {
        inCode = false;
      }
      current += s.slice(i, i + run);
      i += run;
      continue;
    }
    if (ch === "\\" && s[i + 1] === "|") {
      current += "\\|";
      i += 2;
      continue;
    }
    if (ch === "|" && !inCode) {
      cells.push(current.trim());
      current = "";
      i++;
      continue;
    }
    current += ch;
    i++;
  }
  // A trailing pipe leaves an empty remainder that is the border, not a cell.
  if (!(s.endsWith("|") && current.trim() === "")) cells.push(current.trim());
  return cells;
}

/** Display width of a string in a monospace grid: wide glyphs count double. */
export function displayWidth(text: string): number {
  let width = 0;
  const segmenter =
    typeof Intl !== "undefined" && "Segmenter" in Intl
      ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
      : null;
  const graphemes = segmenter
    ? Array.from(segmenter.segment(text), (s) => s.segment)
    : Array.from(text);
  for (const g of graphemes) {
    const cp = g.codePointAt(0) ?? 0;
    if (cp === 0) continue;
    // Combining marks and zero-width joiners take no column of their own.
    if (/^\p{M}+$/u.test(g) || cp === 0x200d) continue;
    width += isWide(cp) || /\p{Extended_Pictographic}/u.test(g) ? 2 : 1;
  }
  return width;
}

/** East Asian Wide/Fullwidth ranges — the ones that matter in practice. */
function isWide(cp: number): boolean {
  return (
    (cp >= 0x1100 && cp <= 0x115f) ||
    (cp >= 0x2e80 && cp <= 0xa4cf) ||
    (cp >= 0xac00 && cp <= 0xd7a3) ||
    (cp >= 0xf900 && cp <= 0xfaff) ||
    (cp >= 0xfe30 && cp <= 0xfe4f) ||
    (cp >= 0xff00 && cp <= 0xff60) ||
    (cp >= 0xffe0 && cp <= 0xffe6) ||
    (cp >= 0x20000 && cp <= 0x3fffd)
  );
}

function alignmentOf(delimiterCell: string): Alignment {
  const c = delimiterCell.trim();
  const left = c.startsWith(":");
  const right = c.endsWith(":");
  if (left && right) return "center";
  if (left) return "left";
  if (right) return "right";
  return "none";
}

function pad(text: string, width: number, align: Alignment): string {
  const gap = Math.max(0, width - displayWidth(text));
  switch (align) {
    case "right":
      return " ".repeat(gap) + text;
    case "center": {
      const before = Math.floor(gap / 2);
      return " ".repeat(before) + text + " ".repeat(gap - before);
    }
    default:
      return text + " ".repeat(gap);
  }
}

function delimiter(width: number, align: Alignment): string {
  // Delimiters are at least three characters so a one-letter column still
  // parses as a table.
  const w = Math.max(3, width);
  switch (align) {
    case "left":
      return ":" + "-".repeat(w - 1);
    case "right":
      return "-".repeat(w - 1) + ":";
    case "center":
      return ":" + "-".repeat(w - 2) + ":";
    default:
      return "-".repeat(w);
  }
}

/**
 * Rewrites a table block so every column is padded to its widest cell.
 * Returns the input unchanged when it is not a table (no delimiter row on
 * the second line), so callers can apply it blindly.
 */
export function formatTable(block: string): string {
  const trailingNewline = block.endsWith("\n") ? "\n" : "";
  const lines = block.replace(/\n$/, "").split("\n");
  if (lines.length < 2 || !isDelimiterLine(lines[1]!)) return block;

  const rows = lines.map(splitCells);
  const aligns = rows[1]!.map(alignmentOf);
  const columns = Math.max(...rows.map((r) => r.length));
  while (aligns.length < columns) aligns.push("none");

  const widths = Array.from({ length: columns }, (_, col) =>
    Math.max(
      3,
      ...rows.map((r, i) => (i === 1 ? 0 : displayWidth(r[col] ?? ""))),
    ),
  );

  const out = rows.map((cells, rowIndex) => {
    const rendered = Array.from({ length: columns }, (_, col) =>
      rowIndex === 1
        ? delimiter(widths[col]!, aligns[col]!)
        : pad(cells[col] ?? "", widths[col]!, aligns[col]!),
    );
    return "| " + rendered.join(" | ") + " |";
  });
  return out.join("\n") + trailingNewline;
}

/**
 * Finds the table containing `offset`, as a line-aligned range. A table is
 * a run of consecutive row-shaped lines whose second line is a delimiter.
 * Returns null when the cursor is not inside one.
 */
export function findTableAt(text: string, offset: number): TableRange | null {
  const lines = text.split("\n");
  // Locate the cursor's line.
  let pos = 0;
  let cursorLine = -1;
  for (let i = 0; i < lines.length; i++) {
    const end = pos + lines[i]!.length;
    if (offset >= pos && offset <= end) {
      cursorLine = i;
      break;
    }
    pos = end + 1;
  }
  if (cursorLine < 0 || !isRowLine(lines[cursorLine]!)) return null;

  let start = cursorLine;
  while (start > 0 && isRowLine(lines[start - 1]!)) start--;
  let end = cursorLine;
  while (end + 1 < lines.length && isRowLine(lines[end + 1]!)) end++;

  // It's only a table if line two of the run is the delimiter.
  if (end - start < 1 || !isDelimiterLine(lines[start + 1]!)) return null;

  const from = lines.slice(0, start).reduce((n, l) => n + l.length + 1, 0);
  const to = from + lines.slice(start, end + 1).join("\n").length;
  return { from, to };
}

/** Reformats every table in a document. */
export function formatAllTables(text: string): string {
  const lines = text.split("\n");
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    if (
      isRowLine(lines[i]!) &&
      i + 1 < lines.length &&
      isDelimiterLine(lines[i + 1]!)
    ) {
      let j = i + 1;
      while (j + 1 < lines.length && isRowLine(lines[j + 1]!)) j++;
      out.push(formatTable(lines.slice(i, j + 1).join("\n")));
      i = j + 1;
      continue;
    }
    out.push(lines[i]!);
    i++;
  }
  return out.join("\n");
}
