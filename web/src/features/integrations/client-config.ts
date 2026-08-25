/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import type { IntegrationModel, IntegrationProtocol } from './model-catalog'

export type ClientPreset =
  | 'curl'
  | 'openai'
  | 'codex'
  | 'claude'
  | 'gemini'
  | 'cherry'
  | 'image'
  | 'video'
  | 'embeddings'
  | 'rerank'

export type ClientTemplate = {
  preset: ClientPreset
  title: string
  description: string
  language: string
  content: string
  endpoint: string
  fileName: string
  install?: string
  path?: string
}

export type ClientTemplateOptions = {
  protocol?: 'chat' | 'responses'
}

const hasProtocol = (
  model: IntegrationModel | undefined,
  protocol: IntegrationProtocol
) => Boolean(model?.protocols.includes(protocol))

export function compatiblePresets(
  model: IntegrationModel | undefined
): ClientPreset[] {
  if (!model) {
    return [
      'curl',
      'openai',
      'codex',
      'claude',
      'gemini',
      'cherry',
      'image',
      'video',
      'embeddings',
      'rerank',
    ]
  }
  const presets: ClientPreset[] = []
  if (hasProtocol(model, 'openai-chat')) {
    presets.push('curl', 'openai', 'cherry')
  }
  if (hasProtocol(model, 'openai-responses')) {
    presets.push('codex')
    if (!hasProtocol(model, 'openai-chat')) presets.push('curl', 'openai')
  }
  if (hasProtocol(model, 'anthropic')) presets.push('claude')
  if (hasProtocol(model, 'gemini') || hasProtocol(model, 'gemini-image')) {
    presets.push('gemini')
  }
  if (hasProtocol(model, 'image')) presets.push('image')
  if (hasProtocol(model, 'video')) presets.push('video')
  if (hasProtocol(model, 'embeddings')) presets.push('embeddings')
  if (hasProtocol(model, 'rerank')) presets.push('rerank')
  if (presets.length === 0) presets.push('curl')
  return presets
}

function jsonString(value: string): string {
  return JSON.stringify(value)
}

export function buildClientTemplate(
  preset: ClientPreset,
  modelName: string,
  apiBaseUrl: string,
  model?: IntegrationModel,
  options?: ClientTemplateOptions
): ClientTemplate {
  const selectedModel = modelName.trim() || 'YOUR_MODEL'
  const chatEndpoint = `${apiBaseUrl}/chat/completions`
  const responsesEndpoint = `${apiBaseUrl}/responses`
  const anthropicEndpoint = `${apiBaseUrl}/messages`
  const origin = apiBaseUrl.replace(/\/v1\/?$/, '')
  const geminiBaseUrl = `${origin}/v1beta`
  const geminiEndpoint = `${geminiBaseUrl}/models/${encodeURIComponent(selectedModel)}:generateContent`
  let usesResponses =
    hasProtocol(model, 'openai-responses') && !hasProtocol(model, 'openai-chat')
  if (options?.protocol === 'responses') {
    usesResponses = true
  } else if (options?.protocol === 'chat') {
    usesResponses = false
  }
  const openaiEndpoint = usesResponses ? responsesEndpoint : chatEndpoint

  if (preset === 'image') {
    const isGeminiImage =
      !hasProtocol(model, 'image') && hasProtocol(model, 'gemini-image')
    const endpoint = isGeminiImage
      ? geminiEndpoint
      : `${apiBaseUrl}/images/generations`
    return {
      preset,
      title: 'Image API',
      description:
        'OpenAI-compatible image generation request for image models.',
      language: 'bash',
      endpoint,
      fileName: isGeminiImage
        ? 'benefit-api-gemini-image.sh'
        : 'benefit-api-image.sh',
      content: isGeminiImage
        ? `curl ${endpoint} \\
	-H "Content-Type: application/json" \\
	-H "x-goog-api-key: YOUR_API_KEY" \\
	-d '{"contents":[{"role":"user","parts":[{"text":"Describe the image you want."}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}'`
        : `curl ${endpoint} \\
	-H "Content-Type: application/json" \\
	-H "Authorization: Bearer YOUR_API_KEY" \\
	-d '{"model":${jsonString(selectedModel)},"prompt":"Describe the image you want.","size":"1024x1024","n":1,"response_format":"b64_json"}'`,
    }
  }

  if (preset === 'video') {
    const endpoint = `${apiBaseUrl}/videos`
    return {
      preset,
      title: 'Video API',
      description: 'OpenAI-compatible asynchronous video generation request.',
      language: 'bash',
      endpoint,
      fileName: 'benefit-api-video.sh',
      content: `curl ${endpoint} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{"model":${jsonString(selectedModel)},"prompt":"Describe the video you want.","seconds":5}'`,
    }
  }

  if (preset === 'gemini') {
    const imageResponse = hasProtocol(model, 'gemini-image')
    return {
      preset,
      title: 'Gemini',
      description: imageResponse
        ? 'Native Gemini generateContent request with image output enabled.'
        : 'Native Gemini generateContent request through the Benefit API.',
      language: 'bash',
      endpoint: geminiEndpoint,
      fileName: 'benefit-api-gemini.sh',
      content: `curl ${geminiEndpoint} \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: YOUR_API_KEY" \\
  -d '{"contents":[{"role":"user","parts":[{"text":"Say hello in one sentence."}]}]${
    imageResponse
      ? ',"generationConfig":{"responseModalities":["TEXT","IMAGE"]}'
      : ''
  }}'`,
    }
  }

  if (preset === 'embeddings') {
    const endpoint = `${apiBaseUrl}/embeddings`
    return {
      preset,
      title: 'Embeddings API',
      description: 'OpenAI-compatible text embedding request.',
      language: 'bash',
      endpoint,
      fileName: 'benefit-api-embeddings.sh',
      content: `curl ${endpoint} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{"model":${jsonString(selectedModel)},"input":"Text to embed."}'`,
    }
  }

  if (preset === 'rerank') {
    const endpoint = `${apiBaseUrl}/rerank`
    return {
      preset,
      title: 'Rerank API',
      description: 'Rerank documents with a model enabled for this token.',
      language: 'bash',
      endpoint,
      fileName: 'benefit-api-rerank.sh',
      content: `curl ${endpoint} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{"model":${jsonString(selectedModel)},"query":"What is relevant?","documents":["First document","Second document"],"top_n":2}'`,
    }
  }

  if (preset === 'codex') {
    return {
      preset,
      title: 'Codex CLI',
      description: 'A ready-to-copy Responses configuration for Codex.',
      language: 'toml',
      endpoint: responsesEndpoint,
      fileName: 'config.toml',
      install: 'npm install -g @openai/codex',
      path: '~/.codex/config.toml',
      content: `model_provider = "benefit_api"
model = ${jsonString(selectedModel)}

[model_providers.benefit_api]
name = "Benefit API"
base_url = ${jsonString(apiBaseUrl)}
wire_api = "responses"
env_key = "BENEFIT_API_KEY"

# PowerShell: $env:BENEFIT_API_KEY = "YOUR_API_KEY"
# macOS/Linux: export BENEFIT_API_KEY="YOUR_API_KEY"`,
    }
  }

  if (preset === 'claude') {
    return {
      preset,
      title: 'Claude Code',
      description:
        'Anthropic-compatible Messages configuration for Claude Code.',
      language: 'bash',
      endpoint: anthropicEndpoint,
      fileName: 'benefit-api-claude.env',
      install: 'npm install -g @anthropic-ai/claude-code',
      path: 'Shell environment',
      content: `# Set these variables in the shell that starts Claude Code
ANTHROPIC_BASE_URL=${apiBaseUrl}
ANTHROPIC_AUTH_TOKEN=YOUR_API_KEY
ANTHROPIC_MODEL=${selectedModel}

# PowerShell
$env:ANTHROPIC_BASE_URL = "${apiBaseUrl}"
$env:ANTHROPIC_AUTH_TOKEN = "YOUR_API_KEY"
$env:ANTHROPIC_MODEL = "${selectedModel}"`,
    }
  }

  if (preset === 'openai') {
    const content = usesResponses
      ? `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="${apiBaseUrl}",
)

response = client.responses.create(
    model=${jsonString(selectedModel)},
    input="Say hello in one sentence.",
)
print(response.output_text)`
      : `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="${apiBaseUrl}",
)

response = client.chat.completions.create(
    model=${jsonString(selectedModel)},
    messages=[{"role": "user", "content": "Say hello in one sentence."}],
)
print(response.choices[0].message.content)`
    return {
      preset,
      title: 'OpenAI SDK',
      description:
        'Official OpenAI Python SDK using the current model endpoint.',
      language: 'python',
      endpoint: openaiEndpoint,
      fileName: 'benefit-api-openai.py',
      install: 'pip install openai',
      content,
    }
  }

  if (preset === 'cherry') {
    return {
      preset,
      title: 'Cherry Studio',
      description: 'OpenAI-compatible provider fields for Cherry Studio.',
      language: 'json',
      endpoint: chatEndpoint,
      fileName: 'benefit-api-cherry.json',
      content: `{
  "apiType": "openai",
  "apiKey": "YOUR_API_KEY",
  "baseURL": ${jsonString(apiBaseUrl)},
  "models": [${jsonString(selectedModel)}]
}`,
    }
  }

  const isResponses = usesResponses
  const endpoint = isResponses ? responsesEndpoint : chatEndpoint
  const body = isResponses
    ? `{"model":${jsonString(selectedModel)},"input":"Say hello in one sentence."}`
    : `{"model":${jsonString(selectedModel)},"messages":[{"role":"user","content":"Say hello in one sentence."}]}`
  return {
    preset,
    title: 'cURL',
    description: 'Test the Benefit API gateway from any terminal.',
    language: 'bash',
    endpoint,
    fileName: 'benefit-api-request.sh',
    content: `curl ${endpoint} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '${body}'`,
  }
}
