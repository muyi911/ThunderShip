import type { IBuffer } from "@xterm/xterm";

export function interactiveInput(buffer: IBuffer, rows: number) {
  if (buffer.type !== "normal" || buffer.viewportY !== buffer.baseY) return null;
  const end = buffer.baseY + buffer.cursorY;
  const last = buffer.getLine(end);
  if (!last || last.translateToString(true, buffer.cursorX).trim()) return null;
  for (let row = end + 1; row < buffer.baseY + rows; row++) {
    if (buffer.getLine(row)?.translateToString(true).trim()) return null;
  }
  let start = end;
  while (start > 0 && end - start < 8 && buffer.getLine(start)?.isWrapped) start--;
  let line = "";
  for (let row = start; row <= end; row++) {
    line += buffer.getLine(row)?.translateToString(false, 0, row === end ? buffer.cursorX : undefined) || "";
  }
  const prompt = /^(?:➜\s{2}\S+\s|.*?[$#%] )/.exec(line);
  return prompt ? { prompt: prompt[0], text: line.slice(prompt[0].length) } : null;
}

export function pathCompletionQuery(line: string) {
  if (/[$`]/.test(line)) return null;
  let start = 0, word = "", quote = "", escaped = false;
  const words: string[] = [];
  for (let index = 0; index < line.length; index++) {
    const char = line[index]!;
    if (escaped) { word += char; escaped = false; continue; }
    if (char === "\\" && quote !== "'") { escaped = true; continue; }
    if (quote) { if (char === quote) quote = ""; else word += char; continue; }
    if (char === "'" || char === '"') { quote = char; continue; }
    if (/[;&|\n]/.test(char)) { words.length = 0; word = ""; start = index + 1; continue; }
    if (/\s/.test(char)) {
      if (word) words.push(word);
      word = ""; start = index + 1;
    } else word += char;
  }
  if (escaped || /[$`]/.test(word) || (!words.length && !word.includes("/"))) return null;
  return { before: line.slice(0, start), word, directoriesOnly: words[0] === "cd" };
}

export function quoteCompletion(path: string): string {
  if (path.startsWith("~/")) return "~/" + (path.length > 2 ? quoteCompletion(path.slice(2)) : "");
  return /^[\w./~@%+:,=-]+$/.test(path) ? path : "'" + path.replaceAll("'", "'\\''") + "'";
}
