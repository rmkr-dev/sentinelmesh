# AI safety

Default posture: read, analyze, recommend.

The system prompt in `prompts/root-cause-analysis/system.md` tells the model to cite evidence, list gaps, avoid secrets, and avoid invented telemetry. The merge function enforces the parts that a prompt cannot guarantee: unknown evidence ids are dropped, and `confirmed` is downgraded when the pack does not already contain a confirmed fact.

Automated remediation is a different package and is off. The model cannot call it.
