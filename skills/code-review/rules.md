# Review Rules

1. Flag hardcoded secrets or tokens as critical.
2. Flag direct panic paths in newly added non-test code as high severity.
3. Flag new functions without a nearby test hint as low confidence warnings.
4. Flag TODO and FIXME markers in new code as medium severity.

