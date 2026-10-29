import * as backend from "../bindings/shell_workflow/app";
import type {
  Config,
  Server,
  Workflow,
  Combination,
  Provider,
  FileMapping,
  Artifact,
  ProbeResult,
  Node,
  Run,
  Event,
  AIRequest,
  Draft,
  ArtifactInput,
  ArtifactResource,
  ConnectionInfo,
  TerminalRead,
  RemoteEntry,
  LocalDirectory,
  WorkflowPackage,
  WorkflowBindings,
} from "./types";

type Backend = {
  GetClipboardFiles(): Promise<string[]>;
  CompleteTerminalPath(id: string, directory: string, word: string, directoriesOnly: boolean): Promise<string[]>;
  UploadRemotePaths(id: string, dir: string, paths: string[], sudo: boolean, executable: boolean): Promise<void>;
  PreviewRemoteUpload(id: string, directory: string, paths: string[], sudo: boolean): Promise<import("./types").UploadPreview[]>;
  UploadRemoteSelection(id: string, files: FileMapping[], sudo: boolean, protocol: string): Promise<void>;
  DownloadRemoteWithProtocol(id: string, remote: string, sudo: boolean, protocol: string): Promise<void>;
  GetTunnelRules(serverID: string): Promise<import("./types").TunnelRule[]>;
  SaveTunnelRule(serverID: string, rule: import("./types").TunnelRule): Promise<import("./types").TunnelRule[]>;
  DeleteTunnelRule(serverID: string, ruleID: string): Promise<import("./types").TunnelRule[]>;
  StartTunnel(connectionID: string, ruleID: string): Promise<import("./types").TunnelStatus>;
  StopTunnel(connectionID: string, ruleID: string): Promise<void>;
  ListTunnels(connectionID: string): Promise<import("./types").TunnelStatus[]>;
  StartCommand(id: string, command: string, directory: string): Promise<string>;
  WriteCommand(id: string, job: string, data: string): Promise<void>;
  ResizeCommand(id: string, job: string, cols: number, rows: number): Promise<void>;
  ReadCommand(id: string, job: string): Promise<import("./types").CommandRead>;
  CancelCommand(id: string, job: string): Promise<void>;
  SuggestTerminalCommand(
    connection: string,
    provider: string,
    prompt: string,
    directory: string,
    output: string,
    request: string,
    images: string[],
  ): Promise<import("./types").TerminalSuggestion>;
  OpenConnection(serverID: string): Promise<ConnectionInfo>;
  ReadServerMetrics(id: string): Promise<import("./types").ServerMetrics>;
  ReadTerminal(id: string): Promise<TerminalRead>;
  WriteTerminal(id: string, data: string): Promise<void>;
  ResizeTerminal(id: string, cols: number, rows: number): Promise<void>;
  CloseConnection(id: string): Promise<void>;
  ListRemote(id: string, dir: string, sudo: boolean): Promise<RemoteEntry[]>;
  ManageRemoteFile(id: string, operation: import("./types").RemoteFileOperation, sudo: boolean): Promise<void>;
  UploadRemote(
    id: string,
    dir: string,
    sudo: boolean,
    folder: boolean,
    executable: boolean,
  ): Promise<void>;
  DownloadRemote(id: string, remote: string, sudo: boolean): Promise<void>;
  ReadRemoteText(id: string, remote: string, sudo: boolean): Promise<import("./types").RemoteText>;
  SaveRemoteText(id: string, remote: string, content: string, version: string, sudo: boolean): Promise<import("./types").RemoteText>;
  GetConfig(): Promise<Config>;
  MoveResource(
    kind: import("./resourceOrder").ResourceKind,
    id: string,
    beforeID: string,
  ): Promise<Config>;
  GetDataDirectory(): Promise<string>;
  OpenDataDirectory(): Promise<void>;
  SaveServer(server: Server, secret: string): Promise<Config>;
  SaveArtifact(asset: ArtifactResource): Promise<Config>;
  ProbeServer(id: string): Promise<ProbeResult>;
  ProbeConnection(server: Server, secret: string): Promise<ProbeResult>;
  TrustHost(id: string, fingerprint: string): Promise<Config>;
  DetectSFTP(id: string): Promise<string[]>;
  SaveWorkflow(workflow: Workflow): Promise<Config>;
  SaveCombination(combination: Combination): Promise<Config>;
  ExportWorkflow(id: string): Promise<boolean>;
  ReadWorkflowImport(): Promise<WorkflowPackage | null>;
  ImportWorkflows(
    bundle: WorkflowPackage,
    bindings: WorkflowBindings,
  ): Promise<Config>;
  Delete(kind: string, id: string): Promise<Config>;
  SaveSettings(concurrency: number, theme: string, uploadProtocol: string): Promise<Config>;
  PickFiles(folder: boolean): Promise<FileMapping[]>;
  ListLocalDirectory(path: string): Promise<LocalDirectory>;
  ResolveLocalPaths(paths: string[]): Promise<FileMapping[]>;
  PickKey(): Promise<string>;
  PreviewFiles(files: FileMapping[]): Promise<Artifact[]>;
  Preflight(root: Node): Promise<string[]>;
  RunInputs(root: Node): Promise<ArtifactInput[]>;
  StartRun(root: Node, name: string): Promise<string>;
  ListRuns(): Promise<Run[]>;
  RunEvents(id: string, after: number): Promise<Event[]>;
  CancelRun(id: string): Promise<void>;
  SaveProvider(provider: Provider, key: string): Promise<Config>;
  ListModels(id: string): Promise<string[]>;
  TestModel(id: string): Promise<string>;
  GenerateDraft(request: AIRequest, requestID: string): Promise<Draft>;
  CancelGeneration(id: string): Promise<void>;
};

export const isDesktop = () => !!(window.chrome?.webview || window.webkit?.messageHandlers?.external);

// The bridge is supplied by Wails. Browser preview never simulates a successful deployment.
export function api<K extends keyof Backend>(
  name: K,
  ...args: Parameters<Backend[K]>
): ReturnType<Backend[K]> {
  const bridge = backend as unknown as Record<string, (...args: unknown[]) => Promise<unknown>>;
  if (!isDesktop() || !bridge[name])
    return Promise.reject(
      new Error("请在霆舟 ThunderShip 桌面窗口中使用此功能"),
    ) as ReturnType<Backend[K]>;
  return bridge[name](...args) as ReturnType<Backend[K]>;
}
export const uid = () => crypto.randomUUID().replaceAll("-", "");
export const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v));
