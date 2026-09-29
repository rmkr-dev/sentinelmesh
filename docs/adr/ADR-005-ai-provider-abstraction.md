# ADR-005 AI provider abstraction

Status: accepted

`ai.Provider` has one method, `Analyze`. OpenAI-compatible and Azure OpenAI implement it. Callers pass an evidence pack. The merge step rejects unsupported evidence ids and refuses to confirm a cause the deterministic engine did not confirm. When `ai.enabled` is false, the process does not construct a provider.
