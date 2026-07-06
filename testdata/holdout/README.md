# Holdout Review Fixtures

These fixtures are self-contained adversarial samples for local acceptance. They are not private hidden data; they are committed holdout cases that exercise variants outside the public fixture matrix.

- `holdout-safe-refactor.diff`: clean helper refactor, expected zero findings.
- `holdout-placeholder-secret.diff`: placeholder secret-like names that should not produce critical findings.
- `holdout-secret-private-key.diff`: private-key shaped secret leak.
- `holdout-lifecycle-combo.diff`: combined context, resource, and database lifecycle risks.
- `model-semantic.diff`: deterministic fake-model semantic signal that proves the model review merge path without a real API key.

