// The one visual an area has: a coloured dot. Used beside the switcher, on
// a document's area chip, and before a title in Browse — small, consistent,
// and never the only carrier of the information (the name is always near).
import { areaColorVar } from "../lib/area.ts";

export function AreaDot({
  color,
  className = "",
  title,
}: {
  color: string | undefined;
  className?: string;
  title?: string;
}) {
  return (
    <span
      aria-hidden={title ? undefined : "true"}
      role={title ? "img" : undefined}
      aria-label={title}
      title={title}
      className={`inline-block size-2 shrink-0 rounded-full ${className}`}
      style={{ background: areaColorVar(color) }}
    />
  );
}
