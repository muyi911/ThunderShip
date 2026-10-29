import { uid } from "./api";
import type { ArtifactResource, FileMapping } from "./types";

const filename = (file: FileMapping) =>
  file.local
    .replace(/[\\/]+$/, "")
    .split(/[\\/]/)
    .pop() ||
  file.name ||
  "app";

export function uploadDestination(file: FileMapping, source = file) {
  return file.includeFolder
    ? file.remote.replace(/\/+$/, "") + "/" + filename(source)
    : file.remote;
}

function mapping(
  file: FileMapping,
  remote: string,
  executable = false,
): FileMapping {
  return {
    id: uid(),
    source: file.id,
    local: "",
    remote,
    executable,
    includeFolder: file.kind === "folder",
  };
}

export function syncUploadMappings(
  asset: ArtifactResource,
  existing: FileMapping[],
) {
  const valid = new Set(asset.files.map((file) => file.id));
  const result = existing.filter((file) => valid.has(file.source || ""));
  for (const file of asset.files) {
    if (!result.some((item) => item.source === file.id))
      result.push(
        mapping(
          file,
          file.kind === "folder" ? "/opt/app" : "/opt/app/" + filename(file),
        ),
      );
  }
  return result;
}

export function addUploadDirectory(
  asset: ArtifactResource,
  existing: FileMapping[],
  directory: string,
) {
  const root =
    directory.trim().replace(/\/+$/, "") ||
    (directory.trim() === "/" ? "/" : "");
  if (
    !root.startsWith("/") ||
    /[\x00\r\n\\]/.test(root) ||
    (root !== "/" &&
      root
        .split("/")
        .slice(1)
        .some((part) => !part || part === "." || part === ".."))
  )
    throw new Error("请输入 Linux 绝对目录，例如 /home/work/service-b");
  const result = [...existing];
  for (const file of asset.files) {
    const remote =
      file.kind === "folder"
        ? root
        : (root === "/" ? "" : root) + "/" + filename(file);
    const added = mapping(
      file,
      remote,
      existing.find((item) => item.source === file.id)?.executable,
    );
    if (
      !result.some(
        (item) =>
          item.source === file.id &&
          uploadDestination(item, file) === uploadDestination(added, file),
      )
    )
      result.push(added);
  }
  return result;
}
