import { computed, ref, watch, type Ref } from "vue";
import { api, clone } from "../api";
import type { Config, Server, Provider, ProbeResult } from "../types";
type Feedback = {
  task: (fn: () => Promise<void>) => Promise<void>;
  notice: Ref<string>;
};
export function useResourceEditors(
  cfg: Ref<Config>,
  view: Ref<string>,
  feedback: Feedback,
  applyTheme: () => void,
) {
  const { task, notice } = feedback;
  const serverDialog = ref(false),
    server = ref<Server>(newServer()),
    secret = ref(""),
    probe = ref<ProbeResult | null>(null),
    testingConnection = ref(false);
  const trustDialog = computed({
    get: () =>
      serverDialog.value &&
      !!probe.value?.fingerprint &&
      !probe.value.ok &&
      !probe.value.changed,
    set: (open: boolean) => {
      if (!open) probe.value = null;
    },
  });
  let probeVersion = 0;
  watch(
    [server, secret, serverDialog],
    () => {
      probeVersion++;
      probe.value = null;
    },
    { deep: true, flush: "sync" },
  );
  watch(
    () => [server.value.host, server.value.port],
    () => {
      const saved = cfg.value.servers.find((s) => s.id === server.value.id);
      server.value.fingerprint =
        saved &&
        saved.host === server.value.host &&
        saved.port === server.value.port
          ? saved.fingerprint
          : "";
    },
    { flush: "sync" },
  );
  watch(
    () => [server.value.auth, server.value.keyPath],
    () => {
      secret.value = "";
    },
    { flush: "sync" },
  );
  const provider = ref<Provider>(newProvider()),
    apiKey = ref(""),
    globalConcurrency = ref(4),
    themeChoice = ref("system"),
    uploadProtocol = ref("sftp");
  function newServer(): Server {
    return {
      id: "",
      name: "",
      group: "",
      host: "",
      port: 22,
      user: "root",
      auth: "password",
      keyPath: "",
      hasSecret: false,
      fingerprint: "",
      machine: "",
      concurrency: 0,
      sudoUpload: false,
      sudoUser: "root",
      sftpPath: "/usr/lib/openssh/sftp-server",
    };
  }
  function newProvider(): Provider {
    return {
      id: "",
      name: "OpenAI",
      baseUrl: "https://api.openai.com/v1",
      model: "",
      mode: "auto",
      allowHttp: false,
      hasSecret: false,
      models: [],
    };
  }
  function editServer(s?: Server) {
    server.value = s ? clone(s) : newServer();
    secret.value = "";
    probe.value = null;
    serverDialog.value = true;
  }
  async function saveServer() {
    await task(async () => {
      const id = server.value.id;
      cfg.value = await api("SaveServer", clone(server.value), secret.value);
      server.value = clone(
        cfg.value.servers.find((s) => s.id === id) || cfg.value.servers.at(-1)!,
      );
      secret.value = "";
      notice.value = "服务器已保存";
      serverDialog.value = false;
    });
  }
  async function probeServer(s: Server) {
    editServer(s);
    await testConnection();
  }
  async function testConnection() {
    if (testingConnection.value) return;
    testingConnection.value = true;
    probe.value = null;
    const version = probeVersion;
    try {
      const result = await api(
        "ProbeConnection",
        clone(server.value),
        secret.value,
      );
      if (version === probeVersion && serverDialog.value) probe.value = result;
    } catch (e) {
      if (version === probeVersion && serverDialog.value)
        probe.value = {
          ok: false,
          changed: false,
          fingerprint: "",
          message: String(e),
        };
    } finally {
      testingConnection.value = false;
    }
  }
  async function trustHost() {
    if (!trustDialog.value || testingConnection.value) return;
    if (!probe.value) return;
    server.value.fingerprint = probe.value.fingerprint;
    await testConnection();
  }
  async function detectSFTP() {
    await task(async () => {
      const paths = await api("DetectSFTP", server.value.id);
      if (paths.length) {
        server.value.sftpPath = paths[0];
        notice.value = "检测到 " + paths.join("、");
      } else notice.value = "未找到常见路径，请填写服务器实际 sftp-server 路径";
    });
  }
  function editProvider(p?: Provider) {
    provider.value = p ? clone(p) : newProvider();
    apiKey.value = "";
    view.value = "settings";
  }
  async function saveProvider() {
    await task(async () => {
      const id = provider.value.id;
      cfg.value = await api(
        "SaveProvider",
        clone(provider.value),
        apiKey.value,
      );
      provider.value = clone(
        cfg.value.providers.find((p) => p.id === id) ||
          cfg.value.providers.at(-1)!,
      );
      apiKey.value = "";
      notice.value = "AI 服务已保存";
    });
  }
  async function getModels() {
    await task(async () => {
      provider.value.models = await api("ListModels", provider.value.id);
      cfg.value = await api("GetConfig");
      notice.value = `已获取 ${provider.value.models.length} 个模型`;
    });
  }
  async function testModel() {
    await task(async () => {
      notice.value = await api("TestModel", provider.value.id);
    });
  }
  async function saveSettings() {
    await task(async () => {
      cfg.value = await api(
        "SaveSettings",
        globalConcurrency.value,
        themeChoice.value,
        uploadProtocol.value,
      );
      applyTheme();
      notice.value = "偏好设置已保存";
    });
  }

  return {
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
  };
}
