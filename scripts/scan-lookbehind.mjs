// 静态扫描构建产物中的正则字面量 lookbehind（Safari < 16.4 解析期崩溃源）。
// 用迷你词法器区分正则字面量与字符串/模板/注释中的 "(?<" 文本，只报告真实正则。
import { readdirSync, statSync, readFileSync } from "node:fs";
import { join } from "node:path";

const root = process.argv[2] ?? "out";
const offenders = [];

function scanFile(path) {
  const src = readFileSync(path, "utf8");
  let i = 0;
  const n = src.length;
  let prevSignificant = ""; // 上一个有意义的 token，用于区分除法与正则
  let line = 1;
  while (i < n) {
    const c = src[i];
    if (c === "\n") { line++; i++; continue; }
    if (c === " " || c === "\t" || c === "\r") { i++; continue; }
    // 注释
    if (c === "/" && src[i + 1] === "/") { while (i < n && src[i] !== "\n") i++; continue; }
    if (c === "/" && src[i + 1] === "*") { i += 2; while (i < n && !(src[i] === "*" && src[i + 1] === "/")) { if (src[i] === "\n") line++; i++; } i += 2; continue; }
    // 字符串
    if (c === '"' || c === "'") {
      const quote = c; i++;
      while (i < n && src[i] !== quote) { if (src[i] === "\\") i++; if (src[i] === "\n") line++; i++; }
      i++; prevSignificant = "str"; continue;
    }
    // 模板字符串（含 ${} 嵌套，按嵌套深度近似处理）
    if (c === "`") {
      let depth = 0; i++;
      while (i < n) {
        if (src[i] === "\\") { i += 2; continue; }
        if (src[i] === "$" && src[i + 1] === "{") { depth++; i += 2; continue; }
        if (src[i] === "}" && depth > 0) { depth--; i++; continue; }
        if (src[i] === "`" && depth === 0) { i++; break; }
        if (src[i] === "\n") line++;
        i++;
      }
      prevSignificant = "str"; continue;
    }
    // 正则字面量：当前字符为 / 且上下文允许正则
    if (c === "/" && regexAllowed(prevSignificant)) {
      const start = i + 1;
      let j = start; let inClass = false;
      while (j < n) {
        const d = src[j];
        if (d === "\\") { j += 2; continue; }
        if (d === "[") inClass = true;
        else if (d === "]") inClass = false;
        else if (d === "/" && !inClass) break;
        else if (d === "\n") break; // 跨行说明不是正则
        j++;
      }
      if (j < n && src[j] === "/") {
        const body = src.slice(start, j);
        if (/\(\?<[=!]/.test(body)) {
          offenders.push(`${path}:${line} regex literal /${body.slice(0, 90)}${body.length > 90 ? "…" : ""}/`);
        }
        i = j + 1;
        // flags
        while (i < n && /[a-z]/.test(src[i])) i++;
        prevSignificant = "regex";
        continue;
      }
      i++; prevSignificant = "/"; continue;
    }
    // 其他 token：累积识别标识符/关键字/括号等
    if (/[A-Za-z_$0-9]/.test(c)) {
      let j = i;
      while (j < n && /[A-Za-z_$0-9]/.test(src[j])) j++;
      prevSignificant = src.slice(i, j);
      i = j;
      continue;
    }
    prevSignificant = c;
    i++;
  }
}

function regexAllowed(prev) {
  if (prev === "") return true;
  if (prev === "str" || prev === "regex") return false; // 字符串后是除法
  if (/^[A-Za-z_$]/.test(prev)) {
    return ["return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "case", "do", "else", "yield", "await"].includes(prev);
  }
  if (/^[0-9]/.test(prev)) return false; // 数字后是除法
  return ["(", ",", ":", "=", "!", "&", "|", "?", "{", "}", ";", "[", "+", "-", "*", "%", "<", ">", "~", "^"].includes(prev) || prev === ")";
  // ) ] 后无法可靠区分，偏向除法（保守：不当作正则起始）
}

function walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) walk(p);
    else if (name.endsWith(".js")) scanFile(p);
  }
}

walk(root);
if (offenders.length === 0) {
  console.log("PASS: no regex-literal lookbehind found in", root);
} else {
  console.log(`FAIL: ${offenders.length} regex-literal lookbehind occurrences:`);
  for (const o of offenders.slice(0, 30)) console.log("  " + o);
  process.exitCode = 1;
}
