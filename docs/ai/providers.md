# Providers

`config/ai/providers.yaml` documents the providers. The process reads `config/environments/*.yaml` and environment variables.

Azure OpenAI:

```yaml
ai:
  enabled: true
  provider: azure-openai
  base_url: https://example.openai.azure.com
  model: gpt-4o
  api_version: "2024-10-21"
  temperature: 0
```

Set `AI_API_KEY` in the environment or Key Vault. Do not put the key in the YAML that you commit.

A local model uses `provider: local` or `openai-compatible` and `AI_BASE_URL=http://host:11434`.
