# Branching

Work is trunk-based. Every pull request targets `main`. Do not open a pull request against another feature branch, and do not stack pull requests.

Branch names:

```text
feat/<area>-<name>
fix/<area>-<name>
docs/<area>-<name>
ci/<area>-<name>
test/<area>-<name>
refactor/<area>-<name>
```

Keep the name free of tool and vendor product names.

## Merges

The owner squash-merges. The squash commit message should be a normal conventional summary of the change. Do not add a generated-by line, a tool co-author trailer, or a tool-named author.

Rebase onto `main` before asking for review so the pull request is a straight line of commits.

## Recommended protection

These settings are for the repository owner. This repository does not change them from a pull request.

- Require a pull request before merging to `main`.
- Require the `ci` jobs to pass, including service tests, Terraform, security, Helm, docs, integration, and commit hygiene once those jobs exist.
- Require a linear history.
- Do not allow force-pushes to `main`.
- Leave stale feature branches for the owner to delete. Do not delete them from a pull request.

`main` currently has no required checks. That is an owner action, not something this tree can turn on.
