import type { Flow, Node, Step, Workflow } from "./types";
import { uid } from "./api";
export const emptyFlow = (): Flow => ({ nodes: [], edges: [] });
export const START_REF = "$start";
export function withStart(flow: Flow): Flow {
  if (flow.nodes.some((n) => n.ref === START_REF)) return flow;
  let id = "start";
  while (flow.nodes.some((n) => n.id === id)) id += "-entry";
  const roots = flow.nodes.filter(
    (n) => !flow.edges.some((e) => e.target === n.id),
  );
  const x = roots[0]?.x ?? 90;
  const y = Math.min(70, ...flow.nodes.map((n) => n.y)) - 150;
  return {
    nodes: [{ id, ref: START_REF, x, y }, ...flow.nodes],
    edges: [...roots.map((n) => ({ source: id, target: n.id })), ...flow.edges],
  };
}
export function canDeleteFrom(target: EventTarget | null) {
  return !(
    target instanceof Element &&
    target.closest(
      'input,textarea,select,[contenteditable="true"],[role="dialog"]',
    )
  );
}
export function workflowServers(w: Workflow): string[] {
  return [
    ...new Set([
      ...(w.servers || []),
      ...w.steps.flatMap((s) => s.targets?.servers || []),
    ]),
  ];
}
export function stepsFlow(steps: Step[]): Flow {
  return {
    nodes: steps.map((s, i) => ({
      id: s.id,
      ref: s.id,
      x: 70,
      y: 70 + i * 130,
    })),
    edges: steps
      .slice(1)
      .map((s, i) => ({ source: steps[i].id, target: s.id })),
  };
}
// Preserve legacy serial/parallel groups when opening an existing composition as a graph.
export function referenceFlow(root: Node): Flow {
  if (root.graph) return root.graph;
  const flow = emptyFlow();
  function visit(n: Node): { heads: string[]; tails: string[] } {
    if (n.workflowId) {
      const id = uid();
      flow.nodes.push({
        id,
        ref: n.workflowId,
        x: 70 + (flow.nodes.length % 3) * 270,
        y: 70 + Math.floor(flow.nodes.length / 3) * 140,
      });
      return { heads: [id], tails: [id] };
    }
    const children = n.children.map(visit);
    if (n.mode === "serial") {
      for (let i = 1; i < children.length; i++)
        for (const source of children[i - 1].tails)
          for (const target of children[i].heads)
            flow.edges.push({ source, target });
      return {
        heads: children[0]?.heads || [],
        tails: children.at(-1)?.tails || [],
      };
    }
    return {
      heads: children.flatMap((c) => c.heads),
      tails: children.flatMap((c) => c.tails),
    };
  }
  visit(root);
  return flow;
}
export function canConnect(
  flow: Flow,
  source: string,
  target: string,
): boolean {
  if (
    flow.nodes.find((n) => n.id === target)?.ref === START_REF ||
    source === target ||
    flow.edges.some((e) => e.source === source && e.target === target)
  )
    return false;
  const seen = new Set<string>();
  function reaches(id: string): boolean {
    if (id === source) return true;
    if (seen.has(id)) return false;
    seen.add(id);
    return flow.edges
      .filter((e) => e.source === id)
      .some((e) => reaches(e.target));
  }
  return !reaches(target);
}
