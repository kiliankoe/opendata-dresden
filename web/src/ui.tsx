// The bits of presentation that more than one view needs

// pill is the shape of the small round-bordered links and buttons
export const pill =
  "inline-block cursor-pointer rounded-full border border-accent px-2.5 py-0.5 text-accent";

// Modes switches between the views of a dataset. The buttons share their
// borders, so the group reads as one control.
export function Modes<T>({
  options,
  active,
  onSelect,
  className = "",
}: {
  options: { label: string; value: T }[];
  active: T | undefined;
  onSelect: (value: T) => void;
  className?: string;
}) {
  return (
    <span className={`flex flex-wrap ${className}`}>
      {options.map(({ label, value }) => (
        <button
          key={label}
          type="button"
          // The active button lies above its neighbors, so that all of its
          // border shows the accent
          className={`cursor-pointer border px-2.5 py-0.5 not-first:-ml-px first:rounded-l-full last:rounded-r-full ${
            value === active
              ? "relative border-accent text-accent"
              : "border-line"
          }`}
          onClick={() => onSelect(value)}
        >
          {label}
        </button>
      ))}
    </span>
  );
}

// Badges labels a dataset with its formats or topics
export function Badges({ items }: { items: string[] }) {
  return (
    <p className="mt-0.5 text-[0.8125rem] text-muted">
      {items.map((item) => (
        <span
          key={item}
          className="mt-1 mr-1 inline-block rounded-sm bg-badge px-1.5"
        >
          {item}
        </span>
      ))}
    </p>
  );
}

// DatesHelp explains the two dates a dataset carries, which read alike but
// come from different places. The bubble opens on hover and on keyboard
// focus.
export function DatesHelp() {
  return (
    <span className="group relative inline-flex align-middle">
      <button
        type="button"
        aria-label="Woher kommen die beiden Daten?"
        className="flex size-4 cursor-help items-center justify-center rounded-full border border-line text-[0.625rem] text-muted"
      >
        ?
      </button>
      <span className="pointer-events-none invisible absolute bottom-full left-0 z-10 mb-1.5 w-56 rounded-md border border-line bg-surface p-2 text-left text-[0.8125rem] leading-snug font-normal text-ink shadow-lg group-focus-within:visible group-hover:visible">
        <b className="font-semibold">Stand</b> gibt das Open-Data-Portal selbst
        an. Es wechselt, sobald die Stadt einen Datensatz neu veröffentlicht,
        auch wenn die Zahlen gleich bleiben.{" "}
        <b className="font-semibold">Geändert</b> ist der Tag, an dem sich die
        Zahlen in der nächtlichen Kopie hier zuletzt wirklich unterschieden
        haben.
      </span>
    </span>
  );
}
