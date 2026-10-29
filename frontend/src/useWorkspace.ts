import { useResourceEditors } from "./composables/useResourceEditors";
import { useWorkflowTransfer } from "./composables/useWorkflowTransfer";
import { useConnectionTabs } from "./composables/useConnectionTabs";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useTheme } from "vuetify";
import { defineStore } from "pinia";
import type {
  Config,
  Workflow,
  Combination,
  Node,
  Step,
  Run,
  Draft,
  ArtifactInput,
} from "./types";
import { api, clone, uid } from "./api";
import { emptyFlow, stepsFlow, referenceFlow, withStart } from "./flow";
import { appendRunEvents, type RunLogEvent } from "./runEvents";
import { orderedResources, type ResourceKind } from "./resourceOrder";
import type { Server } from "./types";
import type { UnwrapNestedRefs } from "vue";

export const useWorkspace = defineStore("workspace", () => {
  const theme = useTheme();
  const cfg = ref<Config>({
    artifacts: [],
    servers: [],
    workflows: [],
    combinations: [],
    providers: [],
    concurrency: 4,
    theme: "system",
  });
  const view = ref("workflows"),
    search = ref(""),
    aiOpen = ref(false),
    treeOpen = ref(true),
    logsOpen = ref(false),
    notice = ref(""),
    fatal = ref(""),
    busy = ref(false);
  const wf = ref<Workflow | null>(null),
    cmb = ref<Combination | null>(null),
    workflowBaseline = ref("null"),
    combinationBaseline = ref("null"),
    unresolved = ref<string[]>([]);
  const pendingDraft = ref<(() => void) | null>(null);
  const missingInputs = ref<ArtifactInput[]>([]);
  const editorKind = ref("workflow");
  const connectionTabs = useConnectionTabs();
  const { connectedServer } = connectionTabs;
  const selected = ref<string[]>([]),
    quickMode = ref("serial"),
    runs = ref<Run[]>([]),
    activeRun = ref(""),
    events = ref<RunLogEvent[]>([]),
    preflight = ref<string[] | null>(null);
  let eventCursor = 0;
  const {
    serverDialog,
    server,
    secret,
    probe,
    trustDialog,
    testingConnection,
    provider,
    apiKey,
    globalConcurrency,
    themeChoice,
    uploadProtocol,
    editServer,
    saveServer,
    probeServer,
    testConnection,
    trustHost,
    detectSFTP,
    editProvider,
    saveProvider,
    getModels,
    testModel,
    saveSettings,
  } = useResourceEditors(cfg, view, { task, notice }, applyTheme);
  const confirmDelete = ref<{ kind: string; id: string; name: string } | null>(
    null,
  );
  const pendingRun = ref<{ node: Node; name: string } | null>(null);
  const workflowTransfer = useWorkflowTransfer(cfg, { task, notice });
  const nav = [
    { id: "workflows", icon: "mdi-file-tree-outline", name: "工作流" },
    { id: "artifacts", icon: "mdi-package-variant-closed", name: "构建物" },
    { id: "servers", icon: "mdi-server-outline", name: "服务器" },
    { id: "runs", icon: "mdi-history", name: "运行记录" },
    { id: "settings", icon: "mdi-cog-outline", name: "设置" },
  ];
  const currentName = computed(
    () => nav.find((n) => n.id === view.value)?.name || "工作流",
  );
  const orderedWorkflows = computed(() =>
    orderedResources(cfg.value.workflows, cfg.value.resourceOrder?.workflows),
  );
  const orderedServers = computed(() =>
    orderedResources(cfg.value.servers, cfg.value.resourceOrder?.servers),
  );
  const orderedArtifacts = computed(() =>
    orderedResources(cfg.value.artifacts, cfg.value.resourceOrder?.artifacts),
  );
  const filteredWorkflows = computed(() =>
    orderedWorkflows.value.filter((w) =>
      w.name.toLowerCase().includes(search.value.toLowerCase()),
    ),
  );
  const filteredServers = computed(() =>
    orderedServers.value.filter((s) =>
      (s.name + s.host + s.group)
        .toLowerCase()
        .includes(search.value.toLowerCase()),
    ),
  );
  const groups = computed(() => [
    ...new Set(filteredServers.value.map((s) => s.group || "未分组")),
  ]);
  const active = computed(() =>
    runs.value.find((r) => r.id === activeRun.value),
  );
  const dirty = computed(
    () =>
      JSON.stringify(
        editorKind.value === "reference" ? cmb.value : wf.value,
      ) !==
      (editorKind.value === "reference"
        ? combinationBaseline.value
        : workflowBaseline.value),
  );
  const root = computed<Node>(() =>
    editorKind.value === "reference" && cmb.value
      ? cmb.value.root
      : { mode: "", workflowId: wf.value?.id || "", children: [] },
  );
  const labels: Record<string, string> = {
    running: "执行中",
    queued: "等待资源",
    success: "成功",
    failed: "失败",
    cancelled: "已取消",
    unknown: "结果未知",
    blocked: "已阻塞",
    uploading: "上传中",
    prepared: "待发布",
    verified: "校验通过",
    published: "上传成功",
    log: "输出",
  };
  function statusColor(s: string) {
    return s === "success"
      ? "success"
      : s === "failed" || s === "unknown"
        ? "error"
        : s === "running" || s === "uploading"
          ? "primary"
          : "grey";
  }
  function resumeDraft(kind: string) {
    editorKind.value = kind;
    view.value = "editor";
  }
  function newStep(type: string): Step {
    return {
      id: uid(),
      name:
        type === "server"
          ? "目标服务器"
          : type === "upload"
            ? "上传文件"
            : type === "workflow"
              ? "运行其他工作流"
              : "执行命令",
      type,
      targets:
        type === "server"
          ? { servers: [], concurrency: 4, resources: [] }
          : undefined,
      artifactId: type === "upload" ? "" : undefined,
      workflowId: type === "workflow" ? "" : undefined,
      files: [],
      command: "",
      commandMode: type === "command" ? "lines" : "",
      commands: type === "command" ? [""] : [],
      directory: "",
      sudo: false,
      timeout: type === "upload" ? 600 : 60,
      interval: 3,
      beforePublish: "",
    };
  }
  function newWorkflow() {
    openWorkflow({
      id: "",
      name: "新建工作流",
      version: 0,
      servers: [],
      concurrency: 4,
      resources: [],
      steps: [],
      flow: emptyFlow(),
    });
  }
  function replaceDraft(kind: string, action: () => void, overwrite = false) {
    const value = kind === "workflow" ? wf.value : cmb.value;
    const baseline =
      kind === "workflow" ? workflowBaseline.value : combinationBaseline.value;
    if (!overwrite && value && JSON.stringify(value) !== baseline) {
      pendingDraft.value = action;
    } else action();
  }
  function discardDraft() {
    const action = pendingDraft.value;
    pendingDraft.value = null;
    action?.();
  }
  function openWorkflow(w: Workflow, overwrite = false) {
    replaceDraft(
      "workflow",
      () => {
        wf.value = clone(w);
        wf.value.flow = withStart(wf.value.flow || stepsFlow(wf.value.steps));
        editorKind.value = "workflow";
        view.value = "editor";
        unresolved.value = [];
        workflowBaseline.value = JSON.stringify(wf.value);
      },
      overwrite,
    );
  }
  function newCombination() {
    openCombination({
      id: "",
      name: "新建编排工作流",
      root: { mode: "", workflowId: "", children: [], graph: emptyFlow() },
    });
  }
  function openCombination(c: Combination, overwrite = false) {
    replaceDraft(
      "combination",
      () => {
        cmb.value = clone(c);
        cmb.value.root = {
          mode: "",
          workflowId: "",
          children: [],
          graph: withStart(referenceFlow(cmb.value.root)),
        };
        editorKind.value = "reference";
        view.value = "editor";
        unresolved.value = [];
        combinationBaseline.value = JSON.stringify(cmb.value);
      },
      overwrite,
    );
  }
  function duplicateWorkflow(source: Workflow | Combination) {
    const copy = clone(source);
    copy.id = "";
    const base = `${source.name} - 副本`;
    const names = new Set(
      [...cfg.value.workflows, ...cfg.value.combinations].map((w) => w.name),
    );
    copy.name = base;
    for (let i = 2; names.has(copy.name); i++) copy.name = `${base} ${i}`;
    if ("steps" in copy) {
      copy.version = 0;
      replaceDraft("workflow", () => {
        openWorkflow(copy, true);
        workflowBaseline.value = "null";
      });
    } else {
      replaceDraft("combination", () => {
        openCombination(copy, true);
        combinationBaseline.value = "null";
      });
    }
  }
  function error(e: unknown) {
    notice.value = String(e instanceof Error ? e.message : e);
  }
  async function task(fn: () => Promise<void>) {
    if (busy.value) return;
    busy.value = true;
    try {
      await fn();
    } catch (e) {
      error(e);
    } finally {
      busy.value = false;
    }
  }
  async function refresh() {
    cfg.value = await api("GetConfig");
    globalConcurrency.value = cfg.value.concurrency;
    themeChoice.value = cfg.value.theme;
    uploadProtocol.value = cfg.value.uploadProtocol || "sftp";
    applyTheme();
  }
  async function moveResource(
    kind: ResourceKind,
    id: string,
    beforeID: string,
  ) {
    await task(async () => {
      cfg.value = await api("MoveResource", kind, id, beforeID);
    });
  }
  function applyTheme() {
    const t =
      cfg.value.theme === "system"
        ? matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : cfg.value.theme;
    theme.change(t);
  }
  async function saveWorkflow() {
    if (!wf.value) return;
    await task(async () => {
      const id = wf.value!.id;
      cfg.value = await api("SaveWorkflow", clone(wf.value!));
      const saved =
        cfg.value.workflows.find((w) => w.id === id) ||
        cfg.value.workflows.at(-1);
      if (saved) openWorkflow(saved, true);
      notice.value = "工作流已保存";
    });
  }
  async function saveCombination() {
    if (!cmb.value) return;
    await task(async () => {
      const id = cmb.value!.id;
      cfg.value = await api("SaveCombination", clone(cmb.value!));
      const saved =
        cfg.value.combinations.find((c) => c.id === id) ||
        cfg.value.combinations.at(-1);
      if (saved) openCombination(saved, true);
      notice.value = "组合已保存";
    });
  }
  async function execute(
    check: boolean,
    n: Node = root.value,
    name = wf.value?.name || cmb.value?.name || "部署组合",
  ) {
    if (busy.value || fatal.value || pendingRun.value) return;
    if (!n.workflowId && !n.children.length && !n.graph?.nodes.length) {
      notice.value = "请先保存并选择工作流";
      return;
    }
    if (!check) {
      pendingRun.value = { node: clone(n), name };
      return;
    }
    await executeNow(true, n, name);
  }
  async function confirmRun() {
    if (!pendingRun.value || busy.value || fatal.value) return;
    const { node, name } = pendingRun.value;
    pendingRun.value = null;
    await executeNow(false, node, name);
  }
  async function executeNow(check: boolean, n: Node, name: string) {
    await task(async () => {
      const inputs = await api("RunInputs", clone(n));
      missingInputs.value = inputs.filter((input) => input.error);
      if (missingInputs.value.length) return;
      if (check) {
        preflight.value = await api("Preflight", clone(n));
      } else {
        activeRun.value = await api("StartRun", clone(n), name);
        events.value = [];
        eventCursor = 0;
        logsOpen.value = true;
        await updateRuns();
        notice.value = "部署已启动";
      }
    });
  }
  function connectServer(s: Server) {
    if (!s.fingerprint) {
      void probeServer(s);
      return;
    }
    connectionTabs.openConnection(s);
    view.value = "connections";
  }
  async function quickRun() {
    const n: Node = {
      mode: quickMode.value,
      workflowId: "",
      children: selected.value.map((id) => ({
        mode: "",
        workflowId: id,
        children: [],
      })),
    };
    await execute(
      false,
      n,
      "临时组合 · " + selected.value.length + " 个工作流",
    );
  }
  function applyDraft(d: Draft, base: string) {
    if (
      JSON.stringify({ workflow: wf.value, combination: cmb.value }) !== base
    ) {
      notice.value = "生成期间草稿已修改，请先核对生成内容，避免覆盖新修改";
      return;
    }
    if (d.workflow) {
      wf.value = clone(d.workflow);
      wf.value.flow ||= stepsFlow(wf.value.steps);
      editorKind.value = "workflow";
      view.value = "editor";
    }
    if (d.combination) {
      cmb.value = clone(d.combination);
      cmb.value.root = {
        mode: "",
        workflowId: "",
        children: [],
        graph: referenceFlow(cmb.value.root),
      };
      if (!d.workflow) {
        editorKind.value = "reference";
        view.value = "editor";
      }
    }
    unresolved.value = [...(d.missing || [])];
    notice.value =
      d.workflow && d.combination
        ? "已应用两个草稿；先保存部署工作流，再从资源栏打开并保存编排草稿"
        : "AI 草稿已应用，请核对差异和命令后保存";
  }
  async function remove() {
    if (!confirmDelete.value) return;
    await task(async () => {
      const d = confirmDelete.value!;
      cfg.value = await api("Delete", d.kind, d.id);
      confirmDelete.value = null;
      selected.value = selected.value.filter((id) =>
        [...cfg.value.workflows, ...cfg.value.combinations].some(
          (w) => w.id === id,
        ),
      );
      if (wf.value?.id === d.id) {
        wf.value = null;
        newWorkflow();
      }
      if (cmb.value?.id === d.id) {
        cmb.value = null;
        newCombination();
      }
      if (d.kind === "provider" && provider.value.id === d.id) editProvider();
      notice.value = "已删除";
    });
  }
  async function updateRuns() {
    runs.value = (await api("ListRuns")).sort((a, b) =>
      b.started.localeCompare(a.started),
    );
    if (activeRun.value) {
      const runId = activeRun.value;
      const rows = events.value;
      const more = await api("RunEvents", runId, eventCursor);
      if (activeRun.value !== runId || events.value !== rows) return;
      appendRunEvents(
        rows,
        more.filter((ev) => ev.seq > eventCursor),
      );
      eventCursor = Math.max(eventCursor, more.at(-1)?.seq || 0);
    }
  }
  function selectRun(r: Run) {
    activeRun.value = r.id;
    events.value = [];
    eventCursor = 0;
    logsOpen.value = true;
    void updateRuns().catch(error);
  }
  let timer: ReturnType<typeof setInterval> | undefined;
  onMounted(async () => {
    try {
      await refresh();
      if (cfg.value.providers.length)
        provider.value = clone(cfg.value.providers[0]);
      await updateRuns();
      timer = setInterval(() => void updateRuns().catch(() => {}), 1500);
    } catch (e) {
      fatal.value = String(e);
    }
  });
  onUnmounted(() => {
    if (timer) clearInterval(timer);
  });
  watch(
    () => view.value,
    () => {
      if (view.value === "connections" && !connectedServer.value)
        view.value = "servers";
    },
  );
  return {
    ...connectionTabs,
    ...workflowTransfer,
    missingInputs,
    resumeDraft,
    editorKind,
    connectedServer,
    connectServer,
    labels,
    theme,
    cfg,
    view,
    wf,
    selected,
    serverDialog,
    provider,
    confirmDelete,
    nav,
    currentName,
    filteredWorkflows,
    orderedServers,
    orderedArtifacts,
    moveResource,
    filteredServers,
    groups,
    active,
    dirty,
    root,
    statusColor,
    newStep,
    newWorkflow,
    openWorkflow,
    newCombination,
    openCombination,
    duplicateWorkflow,
    error,
    task,
    refresh,
    applyTheme,
    saveWorkflow,
    saveCombination,
    execute,
    pendingRun,
    confirmRun,
    quickRun,
    editServer,
    saveServer,
    probeServer,
    testConnection,
    trustHost,
    detectSFTP,
    editProvider,
    saveProvider,
    getModels,
    testModel,
    saveSettings,
    applyDraft,
    remove,
    updateRuns,
    selectRun,
    search,
    aiOpen,
    treeOpen,
    logsOpen,
    notice,
    fatal,
    busy,
    cmb,
    pendingDraft,
    discardDraft,
    unresolved,
    quickMode,
    runs,
    activeRun,
    events,
    preflight,
    server,
    secret,
    probe,
    trustDialog,
    apiKey,
    testingConnection,
    globalConcurrency,
    themeChoice,
    uploadProtocol,
    api,
  };
});
export type Workspace = UnwrapNestedRefs<ReturnType<typeof useWorkspace>>;
