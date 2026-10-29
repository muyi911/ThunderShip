export interface Server {
  id: string;
  name: string;
  group: string;
  host: string;
  port: number;
  user: string;
  auth: string;
  keyPath: string;
  hasSecret: boolean;
  fingerprint: string;
  machine: string;
  concurrency: number;
  sudoUpload: boolean;
  sudoUser: string;
  sftpPath: string;
}
export interface TunnelRule {
  id: string;
  name: string;
  type: "local" | "remote";
  bindHost: string;
  bindPort: number;
  targetHost: string;
  targetPort: number;
}
export interface TunnelStatus {
  id: string;
  running: boolean;
  address: string;
  connections: number;
  error: string;
}
export interface FileMapping {
  id: string;
  source?: string;
  name?: string;
  kind?: string;
  local: string;
  remote: string;
  executable: boolean;
  includeFolder?: boolean;
}
export interface LocalEntry {
  name: string;
  path: string;
  directory: boolean;
  selectable: boolean;
}
export interface LocalDirectory {
  path: string;
  parent: string;
  entries: LocalEntry[];
}
export interface Step {
  id: string;
  name: string;
  type: string;
  targets?: Targets | null;
  artifactId?: string;
  workflowId?: string;
  files: FileMapping[];
  command: string;
  commandMode?: string;
  commands?: string[];
  directory: string;
  sudo: boolean;
  timeout: number;
  interval: number;
  beforePublish: string;
}
export interface Targets {
  servers: string[];
  concurrency: number;
  resources: string[];
}
export interface ArtifactResource {
  id: string;
  name: string;
  files: FileMapping[];
}
export interface ArtifactInput {
  workflowId: string;
  artifactId: string;
  fileId: string;
  workflowName: string;
  name: string;
  kind: string;
  local: string;
  error: string;
}
export interface Workflow {
  flow?: Flow | null;
  id: string;
  name: string;
  version: number;
  servers: string[];
  concurrency: number;
  resources: string[];
  steps: Step[];
}
export interface Node {
  graph?: Flow | null;
  mode: string;
  workflowId: string;
  children: Node[];
}
export interface FlowNode {
  id: string;
  ref: string;
  x: number;
  y: number;
}
export interface FlowEdge {
  source: string;
  target: string;
}
export interface Flow {
  nodes: FlowNode[];
  edges: FlowEdge[];
}
export interface ConnectionInfo {
  id: string;
  directory: string;
}
export interface TerminalRead {
  data: string;
  closed: boolean;
  error: string;
}
export interface CommandRead extends TerminalRead {
  exitCode: number;
  directory: string;
  duration: number;
}
export interface TerminalSuggestion {
  command: string;
  explanation: string;
}
export interface RemoteText {
  content: string;
  version: string;
}
export interface RemoteEntry {
  owner?: string;
  ownerId?: string;
  name: string;
  directory: boolean;
  size: number;
  mode: string;
  modified: string;
  link: boolean;
}
export interface RemoteFileOperation {
  action: "mkdir" | "create" | "rename" | "copy" | "move" | "delete" | "chmod" | "compress" | "extract";
  source: string;
  target: string;
  mode: string;
}
export interface UploadPreview {
  file: FileMapping;
  local: { size: number; mode: string; modified: string };
  remote: { size: number; mode: string; modified: string } | null;
}
export interface Combination {
  id: string;
  name: string;
  root: Node;
}
export interface WorkflowPackage {
  format: string;
  version: number;
  root: string;
  workflows: Workflow[];
  combinations: Combination[];
  servers: { id: string; name: string }[];
  artifacts: ArtifactResource[];
}
export interface WorkflowBindings {
  servers: Record<string, string>;
  artifacts: Record<string, { id: string; files: Record<string, string> }>;
}
export interface Provider {
  id: string;
  name: string;
  baseUrl: string;
  model: string;
  mode: string;
  allowHttp: boolean;
  hasSecret: boolean;
  models: string[];
}
export interface ServerCPU {
  name: string;
  user: number;
  system: number;
  idle: number;
  iowait: number;
  total: number;
}
export interface ServerMetrics {
  timestamp: number;
  hostname: string;
  os: string;
  uptime: number;
  cpuCount: number;
  cpu: ServerCPU;
  cores: ServerCPU[];
  memory: { total: number; available: number; swapTotal: number; swapFree: number };
  networks: { name: string; received: number; sent: number; addresses: string[] }[];
  disks: { source: string; mount: string; total: number; available: number }[];
  processes: { pid: number; name: string; cpu: number; memory: number }[];
  warnings: string[];
}
export interface Config {
  tunnels?: Record<string, TunnelRule[]>;
  resourceOrder?: Partial<
    Record<import("./resourceOrder").ResourceKind, string[]>
  >;
  artifacts: ArtifactResource[];
  servers: Server[];
  workflows: Workflow[];
  combinations: Combination[];
  providers: Provider[];
  concurrency: number;
  theme: string;
  uploadProtocol?: "sftp" | "scp";
}
export interface Draft {
  summary: string;
  missing: string[];
  workflow: Workflow | null;
  combination: Combination | null;
}
export interface Run {
  id: string;
  name: string;
  status: string;
  started: string;
  finished: string;
  error: string;
  root: Node;
}
export interface Event {
  seq: number;
  time: string;
  runId: string;
  workflow: string;
  server: string;
  step: string;
  status: string;
  message: string;
  file: string;
  current: number;
  total: number;
}
export interface Artifact {
  local: string;
  remote: string;
  executable: boolean;
  size: number;
  hash: string;
}
export interface ProbeResult {
  ok: boolean;
  fingerprint: string;
  changed: boolean;
  message: string;
}
export interface AIRequest {
  combination: Combination | null;
  providerId: string;
  prompt: string;
  workflow: Workflow | null;
  serverIds: string[];
  artifactIds: string[];
  workflowIds: string[];
  includeScripts: boolean;
}
