// Loading skeletons for lists — plain pulsing bars, sized to match the dense
// ~32px rows they stand in for.

export function SkeletonRows({ count = 6 }: { count?: number }) {
  return (
    <div aria-hidden="true" className="divide-y divide-border">
      {Array.from({ length: count }, (_, row) => (
        <div key={row} className="flex h-8 items-center gap-3 px-2">
          <div className="size-3.5 animate-pulse rounded-sm bg-hover" />
          <div
            className="h-3 animate-pulse rounded bg-hover"
            style={{ width: `${45 + ((row * 17) % 40)}%` }}
          />
        </div>
      ))}
    </div>
  );
}
