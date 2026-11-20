export type SizeUnits = "binary" | "decimal";

export const formatBytes = (value: number | null, sizeUnits: SizeUnits = "binary") => {
  if (value === null) return "—";
  const decimal = sizeUnits === "decimal";
  const base = decimal ? 1000 : 1024;
  const units = decimal ? ["B", "kB", "MB", "GB", "TB"] : ["B", "KiB", "MiB", "GiB", "TiB"];
  let index = 0;
  while (index < units.length - 1 && value >= base ** (index + 1)) index++;
  return `${(value / base ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
};

export function formatCapacity(gib: number, language: string, sizeUnits?: SizeUnits) {
  if (sizeUnits !== "decimal") return `${gib} GiB`;
  const gb = gib * (1024 / 1000) ** 3;
  return `${new Intl.NumberFormat(language, { maximumFractionDigits: 2, useGrouping: false }).format(gb)} GB`;
}
