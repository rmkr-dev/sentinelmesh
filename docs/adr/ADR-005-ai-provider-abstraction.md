# ADR-005 AI provider abstraction

Status: accepted

`ai.Provider` has one method, `Analyze`. Mock, OpenAI-compatible, and Azure OpenAI implement it. Callers pass an evidence pack. The merge step rejects unsupported evidence ids and refuses to confirm a cause the deterministic engine did not confirm.
