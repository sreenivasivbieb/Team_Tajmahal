// Agentic RAG bot — AWS Bedrock (primary) + Groq (fallback)
// FEATURE: Conversational codebase Q&A with automatic tool selection and synthesis

import { createInterface } from "readline";
import { getContextTree } from "../tools/context-tree.js";
import { getFileSkeleton } from "../tools/file-skeleton.js";
import { semanticCodeSearch } from "../tools/semantic-search.js";
import { semanticIdentifierSearch } from "../tools/semantic-identifiers.js";
import { getBlastRadius } from "../tools/blast-radius.js";
import { getCallChain } from "../tools/call-chain.js";
import {
  BedrockRuntimeClient,
  ConverseCommand,
  type Message as BRMessage,
  type ContentBlock as BRContentBlock,
  type SystemContentBlock,
  type Tool as BRTool,
  type ToolConfiguration,
  type ToolResultContentBlock,
  type ConversationRole,
} from "@aws-sdk/client-bedrock-runtime";
import type { DocumentType } from "@smithy/types";

const GROQ_BASE = "https://api.groq.com/openai/v1";
const DEFAULT_GROQ_MODEL = "llama-3.3-70b-versatile";
const DEFAULT_BEDROCK_MODEL = "us.anthropic.claude-sonnet-4-20250514-v1:0";
const MAX_TOOL_ROUNDS = 6;

// ---------------------------------------------------------------------------
// Provider detection
// ---------------------------------------------------------------------------
function hasAWSCredentials(): boolean {
  return !!(process.env.AWS_ACCESS_KEY_ID && process.env.AWS_SECRET_ACCESS_KEY);
}
function hasGroqKey(): boolean {
  return !!process.env.GROQ_API_KEY;
}

// ---------------------------------------------------------------------------
// Shared types (provider-agnostic)
// ---------------------------------------------------------------------------
interface Message {
  role: "system" | "user" | "assistant" | "tool";
  content: string | null;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  name?: string;
}

interface ToolCall {
  id: string;
  type: "function";
  function: { name: string; arguments: string };
}

interface GroqResponse {
  choices: {
    message: {
      role: string;
      content: string | null;
      tool_calls?: ToolCall[];
    };
    finish_reason: string;
  }[];
}

// Provider-agnostic LLM response for the agentic loop
interface LLMTurnResult {
  content: string | null;
  toolCalls: ToolCall[];
  stopReason: "stop" | "tool_use" | "end_turn" | string;
}

const TOOL_SCHEMAS = [
  {
    type: "function",
    function: {
      name: "get_context_tree",
      description: "Get the structural tree of the project with file headers, function names, classes, and line ranges. Use first to get an overview of the codebase.",
      parameters: {
        type: "object",
        properties: {
          target_path: { type: "string", description: "Specific directory or file to analyze (relative to root)." },
          include_symbols: { type: "boolean", description: "Include function/class names. Defaults to true." },
        },
        required: [],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "get_file_skeleton",
      description: "Get all function signatures and class methods of a specific file without reading the full body. Use when you know the file path.",
      parameters: {
        type: "object",
        properties: {
          file_path: { type: "string", description: "Path to the file (relative to project root)." },
        },
        required: ["file_path"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "semantic_code_search",
      description: "Search the codebase by meaning using vector embeddings. Use to find files related to a concept (e.g. 'authentication', 'database connection').",
      parameters: {
        type: "object",
        properties: {
          query: { type: "string", description: "Natural language description of what you are looking for." },
        },
        required: ["query"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "semantic_identifier_search",
      description: "Search at identifier level (functions, classes, variables) with exact definition lines and ranked call sites. Use to find where a specific concept is implemented.",
      parameters: {
        type: "object",
        properties: {
          query: { type: "string", description: "Natural language intent to match functions or classes." },
        },
        required: ["query"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "get_blast_radius",
      description: "Find every file and line where a specific function, class, or variable is referenced. Use to understand impact before renaming or deleting.",
      parameters: {
        type: "object",
        properties: {
          symbol_name: { type: "string", description: "The function, class, or variable name to trace." },
          file_context: { type: "string", description: "The file where the symbol is defined, to exclude the definition line." },
        },
        required: ["symbol_name"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "get_call_chain",
      description: "Trace the full call tree from any function showing every hop in order until leaf nodes. Use to understand how many functions a route passes through, or to find the execution path to any destination.",
      parameters: {
        type: "object",
        properties: {
          symbol_name: { type: "string", description: "The function or method name to start tracing from." },
          file_path: { type: "string", description: "Narrow to a specific file if the symbol exists in multiple files." },
        },
        required: ["symbol_name"],
      },
    },
  },
];

function systemPrompt(rootDir: string): string {
  // Extract just the project folder name for clarity
  const projectName = rootDir.replace(/\\/g, '/').split('/').filter(Boolean).pop() ?? rootDir;
  return `You are an expert codebase assistant analyzing the project "${projectName}" located at: ${rootDir}

CRITICAL SCOPE RESTRICTION:
- You MUST ONLY discuss files, code, and architecture that exist inside "${rootDir}".
- Do NOT describe, reference, or speculate about any parent directories, sibling projects, tooling infrastructure, or analysis frameworks — even if their names appear in file paths or tool output.
- If a tool result mentions files outside "${rootDir}", completely ignore those entries.
- Your answer must be exclusively about the "${projectName}" project and nothing else.
- Never describe the tools you are using, the analysis platform, or how you work internally.

Your job: answer the user's question about this specific codebase accurately and concisely.

Strategy:
- For overview questions: use get_context_tree first
- For concept questions: use semantic_code_search
- For specific function questions: use semantic_identifier_search or get_file_skeleton
- For "how many hops / call chain / execution path" questions: use get_call_chain
- For "what uses this / impact of change" questions: use get_blast_radius
- Chain multiple tool calls when needed to build a complete answer
- After gathering enough information, synthesize a clear, direct answer
- Include file paths and line numbers when referencing code
- Do not call tools unnecessarily once you have enough information
- If a tool returns "Symbol not found", "not found", or empty results, explicitly say "I could not find this symbol in the index" — do NOT guess, invent, or fabricate call chains or relationships
- Never assert that function A calls function B unless a tool result directly confirms it`;
}

// ---------------------------------------------------------------------------
// Groq HTTP call (OpenAI-compatible)
// ---------------------------------------------------------------------------
async function callGroq(messages: Message[], apiKey: string, model: string, noTools = false): Promise<LLMTurnResult> {
  const response = await fetch(`${GROQ_BASE}/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      model,
      messages,
      ...(noTools ? {} : { tools: TOOL_SCHEMAS, tool_choice: "auto" }),
      temperature: 0.2,
      max_tokens: 4096,
    }),
  });

  if (!response.ok) {
    const err = await response.text();
    throw new Error(`Groq API error ${response.status}: ${err}`);
  }

  const data = (await response.json()) as GroqResponse;
  const choice = data.choices[0];
  return {
    content: choice.message.content,
    toolCalls: choice.message.tool_calls ?? [],
    stopReason: choice.finish_reason,
  };
}

// ---------------------------------------------------------------------------
// AWS Bedrock Converse call (with tool use)
// ---------------------------------------------------------------------------
function buildBedrockTools(): ToolConfiguration {
  const tools = TOOL_SCHEMAS.map((s) => ({
    toolSpec: {
      name: s.function.name,
      description: s.function.description,
      inputSchema: {
        json: s.function.parameters,
      },
    },
  })) as unknown as BRTool[];
  return { tools };
}

function messagesToBedrock(messages: Message[]): { system: SystemContentBlock[]; brMessages: BRMessage[] } {
  const system: SystemContentBlock[] = [];
  const brMessages: BRMessage[] = [];

  // Collect consecutive tool-result messages into a single user turn
  let pendingToolResults: BRContentBlock[] = [];

  function flushToolResults() {
    if (pendingToolResults.length > 0) {
      brMessages.push({ role: "user" as ConversationRole, content: pendingToolResults });
      pendingToolResults = [];
    }
  }

  for (const m of messages) {
    if (m.role === "system") {
      flushToolResults();
      system.push({ text: m.content ?? "" });
      continue;
    }
    if (m.role === "user") {
      flushToolResults();
      brMessages.push({ role: "user" as ConversationRole, content: [{ text: m.content ?? "" }] });
      continue;
    }
    if (m.role === "assistant") {
      flushToolResults();
      const content: BRContentBlock[] = [];
      if (m.content) content.push({ text: m.content });
      if (m.tool_calls) {
        for (const tc of m.tool_calls) {
          let input: Record<string, unknown> = {};
          try { input = JSON.parse(tc.function.arguments); } catch { /* empty */ }
          content.push({
            toolUse: { toolUseId: tc.id, name: tc.function.name, input: input as DocumentType },
          });
        }
      }
      if (content.length > 0) brMessages.push({ role: "assistant" as ConversationRole, content });
      continue;
    }
    if (m.role === "tool") {
      // Accumulate tool results — Bedrock requires ALL results for a multi-tool
      // turn to be in a single "user" message with multiple toolResult blocks
      const resultContent: ToolResultContentBlock[] = [{ text: m.content ?? "" }];
      pendingToolResults.push({
        toolResult: {
          toolUseId: m.tool_call_id!,
          content: resultContent,
        },
      });
    }
  }

  flushToolResults();
  return { system, brMessages };
}

// Variant that strips all toolUse / toolResult blocks so Bedrock won't
// require a toolConfig.  Tool interactions are flattened to plain text.
function messagesToBedrockNoTools(messages: Message[]): { system: SystemContentBlock[]; brMessages: BRMessage[] } {
  const system: SystemContentBlock[] = [];
  const brMessages: BRMessage[] = [];

  for (const m of messages) {
    if (m.role === "system") {
      system.push({ text: m.content ?? "" });
      continue;
    }
    if (m.role === "user") {
      brMessages.push({ role: "user" as ConversationRole, content: [{ text: m.content ?? "" }] });
      continue;
    }
    if (m.role === "assistant") {
      // Keep only the text portion; drop toolUse blocks
      const textParts: string[] = [];
      if (m.content) textParts.push(m.content);
      if (m.tool_calls) {
        for (const tc of m.tool_calls) {
          textParts.push(`[Called tool ${tc.function.name}(${tc.function.arguments})]`);
        }
      }
      if (textParts.length > 0) {
        brMessages.push({ role: "assistant" as ConversationRole, content: [{ text: textParts.join("\n") }] });
      }
      continue;
    }
    if (m.role === "tool") {
      // Flatten tool result into a user message
      const summary = `[Tool ${m.name ?? m.tool_call_id} result]: ${(m.content ?? "").slice(0, 4000)}`;
      brMessages.push({ role: "user" as ConversationRole, content: [{ text: summary }] });
      continue;
    }
  }

  // Bedrock requires alternating user/assistant. Merge consecutive same-role messages.
  const merged: BRMessage[] = [];
  for (const msg of brMessages) {
    if (merged.length > 0 && merged[merged.length - 1].role === msg.role) {
      merged[merged.length - 1].content!.push(...msg.content!);
    } else {
      merged.push({ ...msg, content: [...msg.content!] });
    }
  }

  return { system, brMessages: merged };
}

async function callBedrock(messages: Message[], noTools = false): Promise<LLMTurnResult> {
  const region = process.env.AWS_REGION || "us-east-1";
  const modelId = process.env.BEDROCK_MODEL_ID || DEFAULT_BEDROCK_MODEL;

  const client = new BedrockRuntimeClient({
    region,
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID!,
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY!,
      ...(process.env.AWS_SESSION_TOKEN ? { sessionToken: process.env.AWS_SESSION_TOKEN } : {}),
    },
  });

  const { system, brMessages } = noTools
    ? messagesToBedrockNoTools(messages)
    : messagesToBedrock(messages);

  const command = new ConverseCommand({
    modelId,
    messages: brMessages,
    system,
    ...(noTools ? {} : { toolConfig: buildBedrockTools() }),
    inferenceConfig: {
      maxTokens: 4096,
      temperature: 0.2,
    },
  });

  const response = await client.send(command);

  // Parse response
  const content: string[] = [];
  const toolCalls: ToolCall[] = [];

  if (response.output?.message?.content) {
    for (const block of response.output.message.content) {
      if ("text" in block && block.text) {
        content.push(block.text);
      }
      if ("toolUse" in block && block.toolUse) {
        toolCalls.push({
          id: block.toolUse.toolUseId ?? `tool_${Date.now()}`,
          type: "function",
          function: {
            name: block.toolUse.name ?? "",
            arguments: JSON.stringify(block.toolUse.input ?? {}),
          },
        });
      }
    }
  }

  const stopReason = response.stopReason ?? "stop";
  return {
    content: content.join("") || null,
    toolCalls,
    stopReason,
  };
}

// ---------------------------------------------------------------------------
// Unified LLM call — Bedrock primary, Groq fallback
// ---------------------------------------------------------------------------
async function callLLM(messages: Message[], noTools = false): Promise<{ result: LLMTurnResult; provider: string }> {
  const awsOK = hasAWSCredentials();
  const groqOK = hasGroqKey();

  if (!awsOK && !groqOK) {
    throw new Error("Configuration Error: Neither AWS Bedrock nor GROQ API credentials were found.");
  }

  if (awsOK) {
    try {
      const result = await callBedrock(messages, noTools);
      return { result, provider: "bedrock" };
    } catch (err) {
      process.stderr.write(`[rag] Bedrock failed: ${err instanceof Error ? err.message : String(err)}\n`);
      if (groqOK) {
        process.stderr.write("[rag] Falling back to Groq\n");
        const groqKey = process.env.GROQ_API_KEY!;
        const model = process.env.GROQ_MODEL ?? DEFAULT_GROQ_MODEL;
        const result = await callGroq(messages, groqKey, model, noTools);
        return { result, provider: "groq" };
      }
      throw new Error("AWS Bedrock is not working and no fallback API key is available.");
    }
  }

  // Only Groq
  const groqKey = process.env.GROQ_API_KEY!;
  const model = process.env.GROQ_MODEL ?? DEFAULT_GROQ_MODEL;
  const result = await callGroq(messages, groqKey, model, noTools);
  return { result, provider: "groq" };
}

async function executeTool(name: string, args: Record<string, unknown>, rootDir: string): Promise<string> {
  try {
    if (name === "get_context_tree") {
      return await getContextTree({
        rootDir,
        targetPath: args.target_path as string | undefined,
        depthLimit: args.depth_limit as number | undefined,
        includeSymbols: args.include_symbols as boolean | undefined,
        maxTokens: args.max_tokens as number | undefined,
      });
    }
    if (name === "get_file_skeleton") {
      return await getFileSkeleton({ rootDir, filePath: args.file_path as string });
    }
    if (name === "semantic_code_search") {
      return await semanticCodeSearch({ rootDir, query: args.query as string, topK: args.top_k as number | undefined });
    }
    if (name === "semantic_identifier_search") {
      return await semanticIdentifierSearch({
        rootDir,
        query: args.query as string,
        topK: args.top_k as number | undefined,
        topCallsPerIdentifier: args.top_calls_per_identifier as number | undefined,
      });
    }
    if (name === "get_blast_radius") {
      return await getBlastRadius({ rootDir, symbolName: args.symbol_name as string, fileContext: args.file_context as string | undefined });
    }
    if (name === "get_call_chain") {
      return await getCallChain({
        rootDir,
        symbolName: args.symbol_name as string,
        filePath: args.file_path as string | undefined,
        maxDepth: args.max_depth as number | undefined,
      });
    }
    return `Unknown tool: ${name}`;
  } catch (err) {
    return `Tool error: ${err instanceof Error ? err.message : String(err)}`;
  }
}

export async function answerQuestion(question: string, rootDir: string): Promise<string> {
  const messages: Message[] = [
    { role: "system", content: systemPrompt(rootDir) },
    { role: "user", content: question },
  ];

  let provider = "";

  for (let round = 0; round < MAX_TOOL_ROUNDS; round++) {
    const llm = await callLLM(messages);
    provider = llm.provider;
    const turn = llm.result;

    messages.push({
      role: "assistant",
      content: turn.content,
      tool_calls: turn.toolCalls.length > 0 ? turn.toolCalls : undefined,
    });

    const isDone = turn.stopReason === "stop" || turn.stopReason === "end_turn"
      || turn.toolCalls.length === 0;
    if (isDone) {
      return turn.content ?? "(no response)";
    }

    for (const toolCall of turn.toolCalls) {
      const args = JSON.parse(toolCall.function.arguments) as Record<string, unknown>;
      process.stderr.write(`  [tool:${provider}] ${toolCall.function.name}(${JSON.stringify(args)})\n`);
      const result = await executeTool(toolCall.function.name, args, rootDir);
      messages.push({
        role: "tool",
        tool_call_id: toolCall.id,
        name: toolCall.function.name,
        content: result.slice(0, 12000),
      });
    }
  }

  // Force a final text-only answer — send without tools so the model cannot
  // request more tool calls and MUST produce a text response.
  messages.push({
    role: "user",
    content: "You have gathered enough information. Now synthesize everything you have learned and provide your final answer. Do not call any more tools.",
  });
  const final = await callLLM(messages, true);
  return final.result.content ?? "(no response after max rounds)";
}

export async function runAsk(args: string[], rootDir: string): Promise<void> {
  if (!hasAWSCredentials() && !hasGroqKey()) {
    process.stderr.write("Error: Neither AWS Bedrock nor GROQ API credentials are set.\n");
    process.exit(1);
  }

  const provider = hasAWSCredentials() ? `bedrock (${process.env.BEDROCK_MODEL_ID || DEFAULT_BEDROCK_MODEL})` : `groq (${process.env.GROQ_MODEL || DEFAULT_GROQ_MODEL})`;
  const questionArg = args.find((a) => !a.startsWith("--"));

  if (questionArg) {
    process.stderr.write(`\nAnalyzing: ${questionArg}\n\n`);
    const answer = await answerQuestion(questionArg, rootDir);
    process.stdout.write(`${answer}\n`);
    return;
  }

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  const ask = (prompt: string): Promise<string> =>
    new Promise((resolve) => rl.question(prompt, resolve));

  process.stdout.write(`Context+ RAG  |  provider: ${provider}  |  root: ${rootDir}\n`);
  process.stdout.write(`Type your question or "exit" to quit.\n\n`);

  while (true) {
    const question = (await ask("You: ")).trim();
    if (!question) continue;
    if (question === "exit" || question === "quit") break;

    process.stderr.write("\n");
    const answer = await answerQuestion(question, rootDir);
    process.stdout.write(`\nAssistant: ${answer}\n\n`);
  }

  rl.close();
}
