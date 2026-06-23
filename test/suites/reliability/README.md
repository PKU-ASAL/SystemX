# Reliability Suite

Scope: usually `local` or `full`, depending on the case.

System under test:

- agent spool/WAL;
- outage and recovery drain;
- restart and shutdown behavior;
- backpressure and degraded health behavior.

This suite verifies durability and liveness. It should not decide detection rule quality.
