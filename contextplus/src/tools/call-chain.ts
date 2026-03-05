// Call chain tracer showing full hop order from any function to leaf nodes
// FEATURE: Call graph traversal for tracing execution paths across the codebase

import { readFile } from "fs/promises";
import { walkDirectory } from "../core/walker.js";
import { analyzeFile, flattenSymbols, isSupportedFile } from "../core/parser.js";

export interface CallChainOptions {
  rootDir: string;
  symbolName: string;
  filePath?: string;
  maxDepth?: number;
  markPatterns?: string[];
  format?: "text" | "json";
}

export interface ChainNodeJSON {
  name: string;
  file: string;
  line: number;
  endLine: number;
  signature: string;
  isDbHit: boolean;
  isMarked: boolean;
  isExternal: boolean;
  isCycle: boolean;
  isLeaf: boolean;
  calls: ChainNodeJSON[];
}

export interface CallChainJSON {
  root: ChainNodeJSON;
  stats: { total: number; internal: number; leaves: number; dbHits: number; cycles: number };
  symbolName: string;
  file: string;
  line: number;
  endLine: number;
  signature: string;
  maxDepth: number;
}

interface SymbolEntry {
  name: string;
  kind: string;
  file: string;
  line: number;
  endLine: number;
  signature: string;
  parentName?: string;
}

interface ChainNode {
  name: string;
  file: string;
  line: number;
  endLine: number;
  signature: string;
  isDbHit: boolean;
  isMarked: boolean;
  isExternal: boolean;
  isCycle: boolean;
  isLeaf: boolean;
  calls: ChainNode[];
}

const DEFAULT_MAX_DEPTH = 6;

const CALL_SKIP = new Set([
  "if", "for", "while", "switch", "catch", "function", "async", "return",
  "new", "typeof", "await", "console", "require", "Object", "Array",
  "String", "Number", "Boolean", "JSON", "Math", "Date", "Promise",
  "Error", "Map", "Set", "setTimeout", "setInterval", "parseInt",
  "parseFloat", "isNaN", "isFinite", "encodeURIComponent",
]);

const DB_NAME = [
  /^db$/i,
  /^(prisma|knex|sequelize|mongoose|supabase|typeorm|drizzle|pool|client)$/i,
  /^(query|execute|findOne|findMany|findAll|findById|save|insert|upsert|count|aggregate)$/i,
  /^(transaction|commit|rollback|connect|disconnect)$/i,
];

const DB_LINE = [
  /\bdb\s*[.(]/, /\bprisma\b/, /\bknex\b/, /\bsequelize\b/,
  /\bmongoose\b/, /\bsupabase\b/, /\btypeorm\b/, /\bdrizzle\b/,
  /\.query\s*\(/, /\.execute\s*\(/, /\.findOne\s*\(/, /\.findMany\s*\(/,
  /\.findAll\s*\(/, /\.save\s*\(/, /\.insert\s*\(/, /\.upsert\s*\(/,
  /\.create\s*\(/, /\.update\s*\(/, /\.delete\s*\(/, /\.remove\s*\(/,
  /\.aggregate\s*\(/, /\.transaction\s*\(/, /\.collection\s*\(/,
];

function isDbCall(name: string, bodyLines?: string[]): boolean {
  if (DB_NAME.some((p) => p.test(name))) return true;
  if (!bodyLines) return false;
  return bodyLines.some((l) => DB_LINE.some((p) => p.test(l)));
}

function extractCalls(fileLines: string[], startLine: number, endLine: number): string[] {
  const calls = new Set<string>();
  const pattern = /\b([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(/g;
  const jsxPattern = /<([A-Z][a-zA-Z0-9_$]*)/g;
  for (let i = startLine - 1; i < Math.min(endLine, fileLines.length); i++) {
    let m: RegExpExecArray | null;
    pattern.lastIndex = 0;
    while ((m = pattern.exec(fileLines[i])) !== null) {
      if (!CALL_SKIP.has(m[1])) calls.add(m[1]);
    }
    jsxPattern.lastIndex = 0;
    while ((m = jsxPattern.exec(fileLines[i])) !== null) {
      if (!CALL_SKIP.has(m[1])) calls.add(m[1]);
    }
  }
  return Array.from(calls);
}

function pickBest(entries: SymbolEntry[], fromFile?: string): SymbolEntry {
  const pool = entries.filter(
    (e) => !e.file.includes("test") && !e.file.includes("spec") && !e.file.includes(".demo."),
  );
  const candidates = pool.length > 0 ? pool : entries;
  if (!fromFile) return candidates[0];
  const dir = fromFile.includes("/") ? fromFile.substring(0, fromFile.lastIndexOf("/")) : "";
  return candidates.find((e) => e.file.startsWith(dir)) ?? candidates[0];
}

async function buildIndex(rootDir: string): Promise<{
  symbols: Map<string, SymbolEntry[]>;
  fileLines: Map<string, string[]>;
}> {
  const entries = await walkDirectory({ rootDir, depthLimit: 0 });
  const files = entries.filter((e) => !e.isDirectory && isSupportedFile(e.path));
  const symbols = new Map<string, SymbolEntry[]>();
  const fileLines = new Map<string, string[]>();

  for (const file of files) {
    try {
      const content = await readFile(file.path, "utf-8");
      fileLines.set(file.relativePath, content.split("\n"));
      const analysis = await analyzeFile(file.path);
      for (const sym of flattenSymbols(analysis.symbols)) {
        const entry: SymbolEntry = {
          name: sym.name, kind: sym.kind, file: file.relativePath,
          line: sym.line, endLine: sym.endLine, signature: sym.signature,
          parentName: sym.parentName,
        };
        const list = symbols.get(sym.name) ?? [];
        list.push(entry);
        symbols.set(sym.name, list);
      }
    } catch {
    }
  }

  return { symbols, fileLines };
}

async function traceChain(
  name: string,
  fromFile: string | undefined,
  depth: number,
  maxDepth: number,
  pathStack: Set<string>,
  symbols: Map<string, SymbolEntry[]>,
  fileLines: Map<string, string[]>,
  markPatterns: RegExp[],
): Promise<ChainNode> {
  const entries = symbols.get(name);
  if (!entries || entries.length === 0) {
    return {
      name, file: "", line: 0, endLine: 0, signature: "",
      isDbHit: isDbCall(name), isMarked: false,
      isExternal: true, isCycle: false, isLeaf: true, calls: [],
    };
  }

  const entry = pickBest(entries, fromFile);
  const nodeKey = `${entry.file}:${entry.line}`;

  if (pathStack.has(nodeKey)) {
    return {
      name, file: entry.file, line: entry.line, endLine: entry.endLine,
      signature: entry.signature, isDbHit: false, isMarked: false,
      isExternal: false, isCycle: true, isLeaf: true, calls: [],
    };
  }

  const lines = fileLines.get(entry.file) ?? [];
  const bodyLines = lines.slice(entry.line - 1, entry.endLine);
  const dbHit = isDbCall(name, bodyLines);
  const isMarked = markPatterns.length > 0 && bodyLines.some((l) => markPatterns.some((p) => p.test(l)));

  const node: ChainNode = {
    name, file: entry.file, line: entry.line, endLine: entry.endLine,
    signature: entry.signature, isDbHit: dbHit, isMarked,
    isExternal: false, isCycle: false, isLeaf: false, calls: [],
  };

  if (depth >= maxDepth) {
    node.isLeaf = true;
    return node;
  }

  pathStack.add(nodeKey);
  const callNames = extractCalls(lines, entry.line, entry.endLine);
  for (const callName of callNames) {
    if (callName === name) continue;
    node.calls.push(
      await traceChain(callName, entry.file, depth + 1, maxDepth, pathStack, symbols, fileLines, markPatterns),
    );
  }
  pathStack.delete(nodeKey);

  node.isLeaf = node.calls.length === 0;
  return node;
}

function fmtRange(line: number, endLine: number): string {
  return endLine > line ? `L${line}-L${endLine}` : `L${line}`;
}

function renderNode(node: ChainNode, prefix: string, isLast: boolean, isRoot: boolean): string {
  const connector = isRoot ? "" : (isLast ? "└─ " : "├─ ");
  const childPrefix = isRoot ? "" : (isLast ? "   " : "│  ");

  let label: string;
  if (node.isExternal) {
    label = `${node.name}  [leaf]`;
  } else {
    label = `${node.name}  ${node.file}  ${fmtRange(node.line, node.endLine)}`;
    if (node.isLeaf && !node.isCycle) label += "  [leaf]";
  }
  if (node.isDbHit) label += "  ★ db";
  if (node.isMarked) label += "  ★ marked";
  if (node.isCycle) label += "  ↩ cycle";

  let result = `${prefix}${connector}${label}\n`;
  for (let i = 0; i < node.calls.length; i++) {
    result += renderNode(node.calls[i], prefix + childPrefix, i === node.calls.length - 1, false);
  }
  return result;
}

function countStats(node: ChainNode): { total: number; internal: number; leaves: number; dbHits: number; cycles: number } {
  let total = 1;
  let internal = (!node.isExternal && !node.isCycle && !node.isLeaf) ? 1 : 0;
  let leaves = node.isLeaf ? 1 : 0;
  let dbHits = node.isDbHit ? 1 : 0;
  let cycles = node.isCycle ? 1 : 0;
  for (const c of node.calls) {
    const s = countStats(c);
    total += s.total;
    internal += s.internal;
    leaves += s.leaves;
    dbHits += s.dbHits;
    cycles += s.cycles;
  }
  return { total, internal, leaves, dbHits, cycles };
}

export async function getCallChain(options: CallChainOptions): Promise<string> {
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;
  const markPatterns = (options.markPatterns ?? []).map((p) => new RegExp(p));
  const format = options.format ?? "text";
  const { symbols, fileLines } = await buildIndex(options.rootDir);

  const entries = symbols.get(options.symbolName);
  if (!entries || entries.length === 0) {
    return `Symbol "${options.symbolName}" not found. Use get_context_tree to discover function names.`;
  }

  let startEntry: SymbolEntry;
  if (options.filePath) {
    const norm = options.filePath.replace(/\\/g, "/");
    startEntry = entries.find((e) => e.file === norm || e.file.endsWith(norm)) ?? pickBest(entries);
  } else {
    const nonTest = entries.filter((e) => !e.file.includes("test") && !e.file.includes("spec"));
    if (nonTest.length > 1) {
      return `"${options.symbolName}" found in ${entries.length} files. Narrow with file_path:\n` +
        entries.map((e) => `  ${e.file}  L${e.line}  (${e.kind})`).join("\n");
    }
    startEntry = nonTest.length === 1 ? nonTest[0] : entries[0];
  }

  const lines = fileLines.get(startEntry.file) ?? [];
  const bodyLines = lines.slice(startEntry.line - 1, startEntry.endLine);
  const dbHit = isDbCall(startEntry.name, bodyLines);
  const isMarked = markPatterns.length > 0 && bodyLines.some((l) => markPatterns.some((p) => p.test(l)));

  const root: ChainNode = {
    name: startEntry.name, file: startEntry.file,
    line: startEntry.line, endLine: startEntry.endLine,
    signature: startEntry.signature, isDbHit: dbHit, isMarked,
    isExternal: false, isCycle: false, isLeaf: false, calls: [],
  };

  const pathStack = new Set([`${startEntry.file}:${startEntry.line}`]);
  const callNames = extractCalls(lines, startEntry.line, startEntry.endLine);
  for (const callName of callNames) {
    if (callName === startEntry.name) continue;
    root.calls.push(
      await traceChain(callName, startEntry.file, 1, maxDepth, pathStack, symbols, fileLines, markPatterns),
    );
  }
  root.isLeaf = root.calls.length === 0;

  const stats = countStats(root);

  if (format === "json") {
    const jsonResult: CallChainJSON = {
      root: root as ChainNodeJSON,
      stats,
      symbolName: options.symbolName,
      file: startEntry.file,
      line: startEntry.line,
      endLine: startEntry.endLine,
      signature: startEntry.signature,
      maxDepth,
    };
    return JSON.stringify(jsonResult);
  }

  const out: string[] = [
    `Call chain for "${options.symbolName}" (max depth: ${maxDepth})`,
    `${startEntry.file}  ${fmtRange(startEntry.line, startEntry.endLine)}`,
    `Signature: ${startEntry.signature}`,
    "",
    renderNode(root, "", true, true),
    `${stats.total} node(s) | ${stats.internal} internal hop(s) | ${stats.leaves} leaf(ves) | ${stats.dbHits} db hit(s) | ${stats.cycles} cycle(s)`,
  ];

  return out.join("\n");
}
