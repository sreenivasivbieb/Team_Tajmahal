// Agentic RAG bot using Groq tool-calling over the codebase intelligence tools
// FEATURE: Conversational codebase Q&A with automatic tool selection and synthesis

import { createInterface } from "readline";
import { getContextTree } from "../tools/context-tree.js";
import { getFileSkeleton } from "../tools/file-skeleton.js";
import { semanticCodeSearch } from "../tools/semantic-search.js";
import { semanticIdentifierSearch } from "../tools/semantic-identifiers.js";
import { getBlastRadius } from "../tools/blast-radius.js";
import { getCallChain } from "../tools/call-chain.js";

const GROQ_BASE = "https://api.groq.com/openai/v1";
const DEFAULT_MODEL = "llama-3.3-70b-versatile";
const MAX_TOOL_ROUNDS = 6;

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

async function callGroq(messages: Message[], apiKey: string, model: string): Promise<GroqResponse> {
  const response = await fetch(`${GROQ_BASE}/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      model,
      messages,
      tools: TOOL_SCHEMAS,
      tool_choice: "auto",
      temperature: 0.2,
      max_tokens: 4096,
    }),
  });

  if (!response.ok) {
    const err = await response.text();
    throw new Error(`Groq API error ${response.status}: ${err}`);
  }

  return response.json() as Promise<GroqResponse>;
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

export async function answerQuestion(question: string, rootDir: string, apiKey: string, model: string): Promise<string> {
  const messages: Message[] = [
    { role: "system", content: systemPrompt(rootDir) },
    { role: "user", content: question },
  ];

  for (let round = 0; round < MAX_TOOL_ROUNDS; round++) {
    const response = await callGroq(messages, apiKey, model);
    const choice = response.choices[0];
    const assistantMessage = choice.message;

    messages.push({
      role: "assistant",
      content: assistantMessage.content,
      tool_calls: assistantMessage.tool_calls,
    });

    if (choice.finish_reason === "stop" || !assistantMessage.tool_calls || assistantMessage.tool_calls.length === 0) {
      return assistantMessage.content ?? "(no response)";
    }

    for (const toolCall of assistantMessage.tool_calls) {
      const args = JSON.parse(toolCall.function.arguments) as Record<string, unknown>;
      process.stderr.write(`  [tool] ${toolCall.function.name}(${JSON.stringify(args)})\n`);
      const result = await executeTool(toolCall.function.name, args, rootDir);
      messages.push({
        role: "tool",
        tool_call_id: toolCall.id,
        name: toolCall.function.name,
        content: result.slice(0, 12000),
      });
    }
  }

  const final = await callGroq(messages, apiKey, model);
  return final.choices[0].message.content ?? "(no response after max rounds)";
}

export async function runAsk(args: string[], rootDir: string): Promise<void> {
  const apiKey = process.env.GROQ_API_KEY ?? "";
  if (!apiKey) {
    process.stderr.write("Error: GROQ_API_KEY environment variable is not set.\n");
    process.exit(1);
  }

  const model = process.env.GROQ_MODEL ?? DEFAULT_MODEL;
  const questionArg = args.find((a) => !a.startsWith("--"));

  if (questionArg) {
    process.stderr.write(`\nAnalyzing: ${questionArg}\n\n`);
    const answer = await answerQuestion(questionArg, rootDir, apiKey, model);
    process.stdout.write(`${answer}\n`);
    return;
  }

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  const ask = (prompt: string): Promise<string> =>
    new Promise((resolve) => rl.question(prompt, resolve));

  process.stdout.write(`Context+ RAG  |  model: ${model}  |  root: ${rootDir}\n`);
  process.stdout.write(`Type your question or "exit" to quit.\n\n`);

  while (true) {
    const question = (await ask("You: ")).trim();
    if (!question) continue;
    if (question === "exit" || question === "quit") break;

    process.stderr.write("\n");
    const answer = await answerQuestion(question, rootDir, apiKey, model);
    process.stdout.write(`\nAssistant: ${answer}\n\n`);
  }

  rl.close();
}
