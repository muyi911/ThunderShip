export type ResourceKind = "servers" | "artifacts" | "workflows";

export function orderedResources<T extends { id: string }>(
  items: T[],
  order: string[] = [],
): T[] {
  const rank = new Map(order.map((id, i) => [id, i]));
  return [...items].sort(
    (a, b) =>
      (rank.get(a.id) ?? order.length) - (rank.get(b.id) ?? order.length),
  );
}
