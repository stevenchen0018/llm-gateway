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
  summary: string
  description?: string
  requestBody?: { content: Record<string, { schema: Schema; example?: unknown }> }
  responses: Record<string, Resp>
}

export interface Endpoint { method: string; path: string; op: Operation }

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

export function sample(lang: Lang, base: string, ep: Endpoint, body: unknown, key = 'sk-your-key'): string {
  const url = base + ep.path
  const json = body === undefined ? '' : JSON.stringify(body, null, 2)
  const call = openaiCall[ep.op.operationId]
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
      return `package main\n\nimport (\n\t"fmt"\n\t"io"\n\t"net/http"\n\t"strings"\n)\n\nfunc main() {\n\tbody := strings.NewReader(\`${json}\`)\n\treq, _ := http.NewRequest("${ep.method}", "${url}", ${ep.method === 'GET' ? 'nil' : 'body'})\n\treq.Header.Set("Authorization", "Bearer ${key}")\n\treq.Header.Set("Content-Type", "application/json")\n\n\tresp, err := http.DefaultClient.Do(req)\n\tif err != nil {\n\t\tpanic(err)\n\t}\n\tdefer resp.Body.Close()\n\tout, _ := io.ReadAll(resp.Body)\n\tfmt.Println(resp.StatusCode, resp.Header.Get("X-Request-Id"), string(out))\n}`
    case 'Java':
      return `import java.net.URI;\nimport java.net.http.*;\n\npublic class Demo {\n  public static void main(String[] args) throws Exception {\n    String body = """\n${json}\n        """;\n    HttpRequest req = HttpRequest.newBuilder(URI.create("${url}"))\n        .header("Authorization", "Bearer ${key}")\n        .header("Content-Type", "application/json")\n        .method("${ep.method}", ${ep.method === 'GET' ? 'HttpRequest.BodyPublishers.noBody()' : 'HttpRequest.BodyPublishers.ofString(body)'})\n        .build();\n    HttpResponse<String> resp = HttpClient.newHttpClient().send(req, HttpResponse.BodyHandlers.ofString());\n    System.out.println(resp.statusCode() + " " + resp.body());\n  }\n}`
  }
}
