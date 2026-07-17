# README Refresh Design

## Conclusion

SysArmor will use an English `README.md` and a structurally equivalent
`README.zh-CN.md`. Both files will serve users and contributors without
turning the repository front page into an architecture reference.

## Audience And Scope

The README must let a new visitor answer three questions quickly:

1. What does SysArmor do?
2. How can I run the standalone Agent or local platform?
3. Where are the detailed architecture, deployment, and test documents?

The README describes only capabilities and commands present in the repository.
Protocol details, complete test matrices, operational diagnostics, and design
rationale remain in their owning documents.

## Structure

Both language versions use the same sections and command examples:

1. project identity and language switch;
2. concise project status;
3. core capabilities;
4. a short architecture flow;
5. prerequisites and quick starts for the standalone Agent and local platform;
6. common development commands;
7. links to architecture, deployment, and testing documentation;
8. contribution and license status.

## Content Rules

- Use factual, restrained language and avoid production-readiness claims.
- Describe the Agent as standalone-first and enrollment as explicit.
- Keep commands aligned with current Makefile targets.
- Do not duplicate detailed gRPC, mTLS, persistence, or benchmark contracts.
- Do not claim a contribution policy, security policy, or license that is not
  present in the repository.
- Keep English and Chinese heading order and code examples equivalent.

## Verification

- Every local Markdown link resolves to an existing path.
- Every documented Make target exists.
- English and Chinese heading structures match.
- Markdown formatting and whitespace checks pass.

