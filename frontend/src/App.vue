<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount } from "vue";
import { useWorkspace } from "./useWorkspace";
import AIPanel from "./AIPanel.vue";
import WorkflowLibrary from "./WorkflowLibrary.vue";
import WorkflowPage from "./WorkflowPage.vue";
import ServerPage from "./ServerPage.vue";
import SettingsPage from "./SettingsPage.vue";
import RunHistory from "./RunHistory.vue";
import ResourceTree from "./ResourceTree.vue";
import RunConsole from "./RunConsole.vue";
import ServerDialog from "./ServerDialog.vue";
import ServerWorkspace from "./ServerWorkspace.vue";
import IconButton from "./IconButton.vue";
import ArtifactPage from "./ArtifactPage.vue";
import MissingArtifacts from "./MissingArtifacts.vue";
import WorkflowImportDialog from "./WorkflowImportDialog.vue";
import brandIcon from "./assets/thundership.png";
import WindowControls from "./WindowControls.vue";
import { Window, Events } from "@wailsio/runtime";
let offFileDrop: (() => void) | undefined;
onMounted(() => { offFileDrop = Events.On("files-dropped", (event) => {
  const { x, y, paths } = event.data as { x: number; y: number; paths: string[] };
  const target = document.elementFromPoint(x, y);
  const list = target?.closest('.remote-file-scroll');
  if (!list || !paths.length) return;
  const directory = (target?.closest('[data-remote-directory]') as HTMLElement | null)?.dataset.remoteDirectory;
  list.dispatchEvent(new CustomEvent('local-files-drop', { detail: { paths, directory } }));
}); });
onBeforeUnmount(() => offFileDrop?.());
const workspace = useWorkspace();
const activeNav = computed(() =>
  workspace.view === "editor"
    ? "workflows"
    : workspace.view === "connections"
      ? "servers"
      : workspace.view,
);
const title = computed(() =>
  workspace.view === "editor"
    ? workspace.editorKind === "workflow"
      ? workspace.wf?.name
      : workspace.cmb?.name
    : workspace.view === "connections"
      ? workspace.connectedServer?.name
      : workspace.currentName,
);
function toggleTheme() {
  workspace.themeChoice = workspace.theme.current.dark ? "light" : "dark";
  workspace.globalConcurrency = workspace.cfg.concurrency;
  void workspace.saveSettings();
}
</script>
<template>
  <v-app
    ><div class="desktop-shell">
      <header class="titlebar">
        <div class="titlebar-drag" @dblclick="Window.ToggleMaximise()">
          <div class="brand-mark">
            <img
              :src="brandIcon"
              alt=""
              width="26"
              height="26"
              draggable="false"
            />
          </div>
          <strong>霆舟 ThunderShip</strong>
        </div>
        <v-btn
          v-if="['workflows', 'editor'].includes(workspace.view)"
          size="small"
          :color="workspace.aiOpen ? 'primary' : undefined"
          prepend-icon="mdi-auto-fix"
          title="打开或收起 AI 工作流编排助手"
          @click="workspace.aiOpen = !workspace.aiOpen"
          >AI 助手</v-btn
        ><IconButton
          icon="mdi-theme-light-dark"
          label="切换浅色 / 深色主题"
          :disabled="workspace.busy"
          @click="toggleTheme"
        />
        <WindowControls />
      </header>
      <v-alert v-if="workspace.fatal" type="error" density="compact">{{
        workspace.fatal
      }}</v-alert>
      <div class="desktop-body">
        <nav class="icon-nav">
          <v-tooltip
            v-for="n in workspace.nav"
            :key="n.id"
            :text="n.name"
            location="right"
            ><template #activator="{ props }"
              ><button
                v-bind="props"
                :class="{ active: activeNav === n.id }"
                :aria-label="n.name"
                :title="n.name"
                @click="workspace.view = n.id"
              >
                <v-icon size="24">{{ n.icon }}</v-icon>
              </button></template
            ></v-tooltip
          ><span class="spacer" /><span class="nav-version">0.2</span>
        </nav>
        <ResourceTree
          v-if="
            ['editor', 'connections'].includes(workspace.view) &&
            workspace.treeOpen
          "
          :workspace="workspace"
        />
        <main class="main-workspace">
          <div class="tabbar">
            <IconButton
              v-if="['editor', 'connections'].includes(workspace.view)"
              icon="mdi-dock-left"
              label="显示或隐藏当前页资源栏"
              size="x-small"
              @click="workspace.treeOpen = !workspace.treeOpen"
            />
            <div v-if="workspace.view !== 'connections'" class="active-tab">
              <v-icon size="16">{{
                workspace.nav.find((n) => n.id === activeNav)?.icon
              }}</v-icon
              >{{ title
              }}<span
                v-if="workspace.view === 'editor' && workspace.dirty"
                class="dirty-dot"
              />
            </div>
            <div class="connection-tabs" role="tablist" aria-label="服务器会话">
              <div
                v-for="tab in workspace.connections"
                :key="tab.id"
                class="connection-tab"
                :class="{
                  active:
                    workspace.view === 'connections' &&
                    workspace.activeConnection === tab.id,
                }"
              >
                <button
                  role="tab"
                  :aria-selected="
                    workspace.view === 'connections' &&
                    workspace.activeConnection === tab.id
                  "
                  @click="
                    workspace.activeConnection = tab.id;
                    workspace.view = 'connections';
                  "
                  :title="tab.server.user + '@' + tab.server.host"
                >
                  <v-icon size="15">mdi-console</v-icon>{{ tab.server.name
                  }}<span v-if="tab.number > 1"> · {{ tab.number }}</span>
                </button>
                <IconButton
                  icon="mdi-close"
                  :label="'关闭连接 ' + tab.server.name + ' ' + tab.number"
                  size="x-small"
                  @click="
                    workspace.closeConnectionTab(tab.id);
                    if (
                      !workspace.connections.length &&
                      workspace.view === 'connections'
                    )
                      workspace.view = 'servers';
                  "
                />
              </div>
            </div>
          </div>
          <div
            v-show="workspace.view !== 'connections'"
            class="workspace-scroll"
            :class="{ 'editor-workspace': workspace.view === 'editor' }"
          >
            <WorkflowLibrary
              v-if="workspace.view === 'workflows'"
              :workspace="workspace"
            /><WorkflowPage
              v-else-if="workspace.view === 'editor'"
              :workspace="workspace"
            /><ArtifactPage
              v-else-if="workspace.view === 'artifacts'"
              :workspace="workspace"
            /><ServerPage
              v-else-if="workspace.view === 'servers'"
              :workspace="workspace"
            /><SettingsPage
              v-else-if="workspace.view === 'settings'"
              :workspace="workspace"
            /><RunHistory
              v-else-if="workspace.view === 'runs'"
              :workspace="workspace"
            />
          </div>
          <ServerWorkspace
            v-for="tab in workspace.connections"
            :key="tab.id"
            v-show="
              workspace.view === 'connections' &&
              workspace.activeConnection === tab.id
            "
            :server="tab.server"
            :active="workspace.view === 'connections' && workspace.activeConnection === tab.id"
            :providers="workspace.cfg.providers"
            @error="workspace.error"
          />
          <RunConsole :workspace="workspace" />
        </main>
        <AIPanel
          v-if="
            workspace.aiOpen && ['workflows', 'editor'].includes(workspace.view)
          "
          :config="workspace.cfg"
          :workflow="
            workspace.view === 'editor' && workspace.editorKind === 'workflow'
              ? workspace.wf
              : null
          "
          :combination="
            workspace.view === 'editor' && workspace.editorKind === 'reference'
              ? workspace.cmb
              : null
          "
          :base-state="
            JSON.stringify({
              workflow: workspace.wf,
              combination: workspace.cmb,
            })
          "
          @close="workspace.aiOpen = false"
          @settings="workspace.view = 'settings'"
          @apply="workspace.applyDraft"
          @error="workspace.error"
        />
      </div>
      <footer class="statusbar">
        <span class="status-dot" /><span
          >{{ workspace.cfg.servers.length }} 台服务器</span
        ><span
          >{{
            workspace.cfg.workflows.length + workspace.cfg.combinations.length
          }}
          个工作流</span
        ><span class="spacer" /><button
          title="展开或收起部署日志"
          @click="workspace.logsOpen = !workspace.logsOpen"
        >
          <v-icon size="14">mdi-console</v-icon>执行日志
        </button>
      </footer>
    </div>
    <v-progress-linear
      v-if="workspace.busy"
      indeterminate
      color="primary"
      class="busy-line"
    />
    <v-snackbar
      :model-value="!!workspace.notice"
      @update:model-value="workspace.notice = ''"
      :timeout="7000"
      location="bottom"
      >{{ workspace.notice
      }}<template #actions
        ><v-btn @click="workspace.notice = ''">关闭</v-btn></template
      ></v-snackbar
    >
    <ServerDialog :workspace="workspace" />
    <MissingArtifacts :workspace="workspace" />
    <WorkflowImportDialog :workspace="workspace" />
    <v-dialog
      :model-value="!!workspace.pendingRun"
      @update:model-value="workspace.pendingRun = null"
      max-width="460"
    >
      <v-card title="确认运行工作流">
        <v-card-text>
          确定运行“{{
            workspace.pendingRun?.name
          }}”？确认后将按已保存的配置执行。
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn autofocus @click="workspace.pendingRun = null">取消</v-btn>
          <v-btn
            color="primary"
            variant="flat"
            prepend-icon="mdi-play"
            :disabled="workspace.busy || !!workspace.fatal"
            @click="workspace.confirmRun()"
            >确认运行</v-btn
          >
        </v-card-actions>
      </v-card>
    </v-dialog>
    <v-dialog
      :model-value="!!workspace.pendingDraft"
      @update:model-value="workspace.pendingDraft = null"
      max-width="440"
      ><v-card title="当前草稿有未保存修改"
        ><v-card-text>继续打开会替换当前草稿。</v-card-text
        ><v-card-actions
          ><v-spacer /><v-btn @click="workspace.pendingDraft = null"
            >继续编辑</v-btn
          ><v-btn color="primary" @click="workspace.discardDraft()"
            >放弃修改并打开</v-btn
          ></v-card-actions
        ></v-card
      ></v-dialog
    >
    <v-dialog
      :model-value="workspace.preflight !== null"
      @update:model-value="workspace.preflight = null"
      max-width="720"
      ><v-card title="预检查"
        ><v-card-text
          ><v-alert
            v-if="!workspace.preflight?.length"
            type="success"
            variant="tonal"
            >连接与文件映射检查通过。</v-alert
          ><v-alert v-else type="error" variant="tonal"
            ><div v-for="issue in workspace.preflight" :key="issue">
              {{ issue }}
            </div></v-alert
          ></v-card-text
        ><v-card-actions
          ><v-spacer /><v-btn @click="workspace.preflight = null"
            >关闭</v-btn
          ></v-card-actions
        ></v-card
      ></v-dialog
    >
    <v-dialog
      :model-value="!!workspace.confirmDelete"
      @update:model-value="workspace.confirmDelete = null"
      max-width="440"
      ><v-card title="删除资源"
        ><v-card-text
          >确定删除“{{ workspace.confirmDelete?.name }}”？</v-card-text
        ><v-card-actions
          ><v-spacer /><v-btn @click="workspace.confirmDelete = null"
            >取消</v-btn
          ><v-btn
            color="error"
            :disabled="workspace.busy"
            @click="workspace.remove()"
            >删除</v-btn
          ></v-card-actions
        ></v-card
      ></v-dialog
    >
  </v-app>
</template>
