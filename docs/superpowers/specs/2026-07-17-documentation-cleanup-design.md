# Documentation Cleanup Design

## Conclusion

SysArmor documentation will describe the current system from first principles.
Each fact has one authoritative owner, nearby README files contain only local
usage information, and completed implementation history is removed after its
durable conclusions are absorbed.

## Information Architecture

- Root README files explain the product, quick starts, and documentation paths.
- `docs/architecture/` explains stable component boundaries and data semantics.
- `deployments/` and `docs/operations/` explain installation and operation.
- `test/` explains validation goals, environments, inputs, outputs, and limits.
- Component README files explain only behavior unique to their directory.

Documents must begin with the problem or decision they explain. File lists,
command inventories, and API field tables are included only when they help a
reader perform a concrete task.

## History Policy

Completed implementation plans are deleted. Design documents are deleted after
their current decisions are represented by formal architecture, deployment, or
test documentation. Git remains the source for implementation history.

Proposed behavior is not mixed into current-state documentation. A design may
remain only while it contains an unimplemented decision that is still actively
maintained and cannot be expressed as a current contract.

## Verification

- All relative Markdown links resolve.
- Documented Make targets and repository paths exist.
- Legacy Agent paths, configuration keys, and removed package names are absent.
- Public API examples match current handlers and types.
- Root and subsystem navigation has no dead ends or duplicate authorities.

