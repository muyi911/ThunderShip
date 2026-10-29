import type { Event } from "./types";

export type RunLogEvent = Event & { rowId: number };
const transferKey = (ev: Event) =>
  JSON.stringify([ev.runId, ev.workflow, ev.server, ev.step, ev.file]);
const pending = (ev: Event) =>
  ev.status === "uploading" || ev.status === "prepared" || ev.status === "verified";

export function appendRunEvents(rows: RunLogEvent[], incoming: Event[]) {
  const transfers = new Map<string, RunLogEvent>();
  for (const row of rows)
    if (row.file && pending(row)) transfers.set(transferKey(row), row);
  for (const ev of incoming) {
    if (ev.file && ["uploading", "prepared", "verified", "published"].includes(ev.status)) {
      const key = transferKey(ev);
      let row = transfers.get(key);
      if (row) Object.assign(row, ev);
      else {
        row = { ...ev, rowId: ev.seq };
        rows.push(row);
      }
      if (pending(ev)) transfers.set(key, row);
      else transfers.delete(key);
      continue;
    }
    if (["failed", "unknown", "cancelled", "blocked"].includes(ev.status)) {
      for (const [key, row] of transfers) {
        if (
          row.runId === ev.runId &&
          (!ev.workflow || row.workflow === ev.workflow) &&
          (!ev.server || row.server === ev.server) &&
          (!ev.step || row.step === ev.step)
        ) {
          Object.assign(row, {
            time: ev.time,
            status: ev.status,
            message: ev.message,
          });
          transfers.delete(key);
        }
      }
    }
    rows.push({ ...ev, rowId: ev.seq });
  }
}
