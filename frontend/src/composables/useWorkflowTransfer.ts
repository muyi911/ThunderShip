import { computed, ref, type Ref } from "vue";
import { api, clone } from "../api";
import type { Config, WorkflowPackage, WorkflowBindings } from "../types";

export function useWorkflowTransfer(
  cfg: Ref<Config>,
  feedback: {
    task: (fn: () => Promise<void>) => Promise<void>;
    notice: Ref<string>;
  },
) {
  const importPackage = ref<WorkflowPackage | null>(null);
  const importBindings = ref<WorkflowBindings>({ servers: {}, artifacts: {} });
  const importReady = computed(() => {
    const p = importPackage.value;
    if (!p) return false;
    return (
      p.servers.every((s) =>
        cfg.value.servers.some(
          (local) => local.id === importBindings.value.servers[s.id],
        ),
      ) &&
      p.artifacts.every((asset) => {
        const binding = importBindings.value.artifacts[asset.id];
        const local = cfg.value.artifacts.find((a) => a.id === binding?.id);
        return (
          !!local &&
          asset.files.every((file) =>
            local.files.some(
              (f) => f.id === binding.files[file.id] && f.kind === file.kind,
            ),
          )
        );
      })
    );
  });
  async function exportWorkflow(id: string) {
    await feedback.task(async () => {
      if (await api("ExportWorkflow", id))
        feedback.notice.value = "工作流已导出，包含引用的子工作流";
    });
  }
  async function openWorkflowImport() {
    await feedback.task(async () => {
      const p = await api("ReadWorkflowImport");
      if (!p) return;
      importBindings.value = {
        servers: Object.fromEntries(p.servers.map((s) => [s.id, ""])),
        artifacts: Object.fromEntries(
          p.artifacts.map((a) => [a.id, { id: "", files: {} }]),
        ),
      };
      importPackage.value = p;
    });
  }
  function bindImportArtifact(requiredID: string, localID: string | null) {
    const required = importPackage.value?.artifacts.find(
      (a) => a.id === requiredID,
    );
    const local = cfg.value.artifacts.find((a) => a.id === localID);
    const files: Record<string, string> = {};
    for (const file of required?.files || []) {
      const matches =
        local?.files.filter(
          (f) => f.kind === file.kind && f.name === file.name,
        ) || [];
      if (matches.length === 1) files[file.id] = matches[0]!.id;
    }
    importBindings.value.artifacts[requiredID] = { id: localID || "", files };
  }
  async function finishWorkflowImport() {
    if (!importReady.value || !importPackage.value) return;
    await feedback.task(async () => {
      cfg.value = await api(
        "ImportWorkflows",
        clone(importPackage.value!),
        clone(importBindings.value),
      );
      importPackage.value = null;
      feedback.notice.value =
        "工作流已导入为新工作流，请核对远端目录和命令后运行";
    });
  }
  return {
    importPackage,
    importBindings,
    importReady,
    exportWorkflow,
    openWorkflowImport,
    bindImportArtifact,
    finishWorkflowImport,
  };
}
