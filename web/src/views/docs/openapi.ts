// Minimal OpenAPI 3 helpers for the docs page: resolve $ref and flatten
// schemas into parameter rows.

export type Schema = {
  $ref?: string
  type?: string
  description?: string
  enum?: string[]
  required?: string[]
  properties?: Record<string, Schema>
  items?: Schema
  oneOf?: Schema[]
  minimum?: number
  maximum?: number
  default?: unknown
}

export interface Spec {
  info: { title: string; version: string; description: string }
  servers: { url: string }[]
  paths: Record<string, Record<string, Operation>>
  components: { schemas: Record<string, Schema>; responses: Record<string, Resp>; securitySchemes: Record<string, { description?: string }> }
}

export interface Resp { $ref?: string; description: string; headers?: Record<string, unknown>; content?: Record<string, { schema?: Schema; example?: unknown }> }

export interface Operation {
  tags?: string[]
  operationId: string
  servers?: { url: string }[]
  parameters?: { name: string; in: string; required?: boolean; description?: string; example?: string }[]
  'x-protocol'?: Protocol
  summary: string
  description?: string
  requestBody?: { content: Record<string, { schema: Schema; example?: unknown }> }
  responses: Record<string, Resp>
}

export type Protocol = 'openai' | 'anthropic' | 'gemini'
export interface Endpoint { method: string; path: string; op: Operation }

export const protocolOf = (ep: Endpoint): Protocol => ep.op['x-protocol'] ?? 'openai'

// fill {param} from the parameter examples (Gemini puts the model in the path)
export const concretePath = (ep: Endpoint, values: Record<string, string> = {}) =>
  ep.path.replace(/\{(\w+)\}/g, (_, n) => values[n] ?? ep.op.parameters?.find((p) => p.name === n)?.example ?? n)

// auth headers each protocol's SDK sends
export function authHeaders(proto: Protocol, key: string): Record<string, string> {
  if (proto === 'anthropic') return { 'x-api-key': key, 'anthropic-version': '2023-06-01' }
  if (proto === 'gemini') return { 'x-goog-api-key': key }
  return { Authorization: `Bearer ${key}` }
}

export interface Row { name: string; type: string; required: boolean; description: string }

export function resolve(spec: Spec, s?: Schema): Schema | undefined {
  if (!s?.$ref) return s
  const name = s.$ref.split('/').pop()!
  return resolve(spec, spec.components.schemas[name])
}

export function resolveResp(spec: Spec, r: Resp): Resp {
  if (!r.$ref) return r
  return spec.components.responses[r.$ref.split('/').pop()!]
}

function typeOf(spec: Spec, s?: Schema): string {
  const r = resolve(spec, s)
  if (!r) return 'any'
  if (r.oneOf) return r.oneOf.map((x) => typeOf(spec, x)).join(' | ')
  if (r.type === 'array') return `${typeOf(spec, r.items)}[]`
  if (r.enum) return `${r.type ?? 'string'} (${r.enum.join(' / ')})`
  return r.type ?? (r.properties ? 'object' : 'any')
}

// flatten turns a schema into dotted rows: choices[].message.content …
export function flatten(spec: Spec, s: Schema | undefined, prefix = '', depth = 0): Row[] {
  const r = resolve(spec, s)
  if (!r || depth > 5) return []
  const obj = r.type === 'array' ? resolve(spec, r.items) : r
  if (!obj?.properties) return []
  const rows: Row[] = []
  for (const [name, prop] of Object.entries(obj.properties)) {
    const p = resolve(spec, prop)
    const full = prefix ? `${prefix}.${name}` : name
    const extras: string[] = []
    if (p?.minimum !== undefined || p?.maximum !== undefined) extras.push(`范围 ${p?.minimum ?? '-∞'} ~ ${p?.maximum ?? '∞'}`)
    if (p?.default !== undefined) extras.push(`默认 ${JSON.stringify(p.default)}`)
    rows.push({ name: full, type: typeOf(spec, prop), required: !!obj.required?.includes(name), description: [p?.description ?? '', ...extras].filter(Boolean).join('；') })
    const child = p?.type === 'array' ? resolve(spec, p.items) : p
    if (child?.properties) rows.push(...flatten(spec, child, p?.type === 'array' ? `${full}[]` : full, depth + 1))
  }
  return rows
}

export function endpoints(spec: Spec): Endpoint[] {
  const out: Endpoint[] = []
  for (const [path, ops] of Object.entries(spec.paths)) for (const [method, op] of Object.entries(ops)) out.push({ method: method.toUpperCase(), path, op })
  return out
}

// ---- code samples ------------------------------------------------------------------

export const LANGS = ['cURL', 'Python', 'Node.js', 'Go', 'Java'] as const
export type Lang = (typeof LANGS)[number]

const openaiCall: Record<string, { py: string; js: string }> = {
  createChatCompletion: { py: 'client.chat.completions.create', js: 'client.chat.completions.create' },
  createCompletion: { py: 'client.completions.create', js: 'client.completions.create' },
  createEmbedding: { py: 'client.embeddings.create', js: 'client.embeddings.create' },
}

function pyValue(v: unknown, indent = 4): string {
  const pad = ' '.repeat(indent)
  if (Array.isArray(v)) return `[\n${v.map((x) => pad + '    ' + pyValue(x, indent + 4)).join(',\n')}\n${pad}]`
  if (v && typeof v === 'object') return `{${Object.entries(v).map(([k, x]) => `"${k}": ${pyValue(x, indent)}`).join(', ')}}`
  if (typeof v === 'boolean') return v ? 'True' : 'False'
  return JSON.stringify(v)
}

// root is the gateway origin; Anthropic and Gemini SDKs take it as base URL
function protocolSdkSample(lang: Lang, proto: Protocol, root: string, ep: Endpoint, body: Record<string, unknown>, key: string): string | null {
  if (proto === 'anthropic' && ep.op.operationId === 'createMessage') {
    if (lang === 'Python') return `import anthropic\n\n# base_url 为网关根地址（SDK 自动拼接 /v1/messages）\nclient = anthropic.Anthropic(base_url="${root}", api_key="${key}")\n\nmsg = client.messages.create(\n${Object.entries(body).map(([k, v]) => `    ${k}=${pyValue(v)},`).join('\n')}\n)\nprint(msg.content[0].text)`
    if (lang === 'Node.js') return `import Anthropic from "@anthropic-ai/sdk";\n\nconst client = new Anthropic({ baseURL: "${root}", apiKey: "${key}" });\n\nconst msg = await client.messages.create(${JSON.stringify(body, null, 2)});\nconsole.log(msg.content[0].text);\n\n// Claude Code：export ANTHROPIC_BASE_URL=${root} ANTHROPIC_AUTH_TOKEN=${key}`
  }
  if (proto === 'gemini' && ep.op.operationId !== 'geminiEmbedContent') {
    const model = ep.op.parameters?.find((p) => p.name === 'model')?.example ?? 'gemini-2.5-flash'
    const text = ((body.contents as { parts: { text: string }[] }[] | undefined)?.[0]?.parts?.[0]?.text) ?? '你好'
    const stream = ep.op.operationId === 'geminiStreamGenerateContent'
    if (lang === 'Python') return `from google import genai\nfrom google.genai import types\n\n# base_url 为网关根地址（SDK 自动拼接 /v1beta/models/...）\nclient = genai.Client(api_key="${key}", http_options=types.HttpOptions(base_url="${root}"))\n\n` +
      (stream ? `for chunk in client.models.generate_content_stream(model="${model}", contents="${text}"):\n    print(chunk.text, end="")` : `resp = client.models.generate_content(model="${model}", contents="${text}")\nprint(resp.text)`)
    if (lang === 'Node.js') return `import { GoogleGenAI } from "@google/genai";\n\nconst ai = new GoogleGenAI({ apiKey: "${key}", httpOptions: { baseUrl: "${root}" } });\n\n` +
      (stream ? `const stream = await ai.models.generateContentStream({ model: "${model}", contents: "${text}" });\nfor await (const chunk of stream) process.stdout.write(chunk.text ?? "");` : `const resp = await ai.models.generateContent({ model: "${model}", contents: "${text}" });\nconsole.log(resp.text);`)
  }
  return null
}

export function sample(lang: Lang, base: string, ep: Endpoint, body: unknown, key = 'sk-your-key'): string {
  const proto = protocolOf(ep)
  const opBase = ep.op.servers?.[0]?.url ?? base
  const url = opBase + concretePath(ep) + (ep.op.operationId === 'geminiStreamGenerateContent' ? '?alt=sse' : '')
  const json = body === undefined ? '' : JSON.stringify(body, null, 2)
  const call = openaiCall[ep.op.operationId]
  const headers = authHeaders(proto, key)
  if (proto !== 'openai') {
    const root = new URL(opBase).origin
    const sdk = protocolSdkSample(lang, proto, root, ep, (body ?? {}) as Record<string, unknown>, key)
    if (sdk) return sdk
    const hdr = Object.entries(headers)
    switch (lang) {
      case 'cURL':
        return `curl ${url} \\\n${hdr.map(([k, v]) => `  -H "${k}: ${v}" \\`).join('\n')}\n  -H "Content-Type: application/json" \\\n  -d '${json}'`
      case 'Python':
        return `import requests\n\nresp = requests.post(\n    "${url}",\n    headers=${JSON.stringify(headers)},\n    json=${pyValue(body)},\n)\nprint(resp.status_code, resp.json())`
      case 'Node.js':
        return `const resp = await fetch("${url}", {\n  method: "POST",\n  headers: ${JSON.stringify({ ...headers, 'Content-Type': 'application/json' })},\n  body: JSON.stringify(${json}),\n});\nconsole.log(resp.status, await resp.json());`
    }
  }
  const setHeaders = Object.entries(headers)
  switch (lang) {
    case 'cURL':
      return ep.method === 'GET'
        ? `curl ${url} \\\n  -H "Authorization: Bearer ${key}"`
        : `curl ${url} \\\n  -H "Authorization: Bearer ${key}" \\\n  -H "Content-Type: application/json" \\\n  -d '${json}'`
    case 'Python':
      if (!call) return `from openai import OpenAI\n\nclient = OpenAI(base_url="${base}", api_key="${key}")\nfor m in client.models.list():\n    print(m.id)`
      return `from openai import OpenAI\n\nclient = OpenAI(base_url="${base}", api_key="${key}")\n\nresp = ${call.py}(\n${Object.entries(body as object).map(([k, v]) => `    ${k}=${pyValue(v)},`).join('\n')}\n)\nprint(resp)`
    case 'Node.js':
      if (!call) return `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${base}", apiKey: "${key}" });\nconst models = await client.models.list();\nconsole.log(models.data.map((m) => m.id));`
      return `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${base}", apiKey: "${key}" });\n\nconst resp = await ${call.js}(${json.replace(/\n/g, '\n')});\nconsole.log(resp);`
    case 'Go':
      return `package main\n\nimport (\n\t"fmt"\n\t"io"\n\t"net/http"\n\t"strings"\n)\n\nfunc main() {\n\tbody := strings.NewReader(\`${json}\`)\n\treq, _ := http.NewRequest("${ep.method}", "${url}", ${ep.method === 'GET' ? 'nil' : 'body'})\n${setHeaders.map(([k, v]) => `\treq.Header.Set("${k}", "${v}")\n`).join('')}\treq.Header.Set("Content-Type", "application/json")\n\n\tresp, err := http.DefaultClient.Do(req)\n\tif err != nil {\n\t\tpanic(err)\n\t}\n\tdefer resp.Body.Close()\n\tout, _ := io.ReadAll(resp.Body)\n\tfmt.Println(resp.StatusCode, resp.Header.Get("X-Request-Id"), string(out))\n}`
    case 'Java':
      return `import java.net.URI;\nimport java.net.http.*;\n\npublic class Demo {\n  public static void main(String[] args) throws Exception {\n    String body = """\n${json}\n        """;\n    HttpRequest req = HttpRequest.newBuilder(URI.create("${url}"))\n${setHeaders.map(([k, v]) => `        .header("${k}", "${v}")\n`).join('')}        .header("Content-Type", "application/json")\n        .method("${ep.method}", ${ep.method === 'GET' ? 'HttpRequest.BodyPublishers.noBody()' : 'HttpRequest.BodyPublishers.ofString(body)'})\n        .build();\n    HttpResponse<String> resp = HttpClient.newHttpClient().send(req, HttpResponse.BodyHandlers.ofString());\n    System.out.println(resp.statusCode() + " " + resp.body());\n  }\n}`
  }
}
