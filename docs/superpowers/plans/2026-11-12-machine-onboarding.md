# Machine onboarding and removal

Goal: machine rows provide usable native-binary installation/rebinding commands, explicit credential rotation, and safe removal. Node hosts do not require Docker or a compiler.

Design: reuse Caddy's native installer and versioned GitHub Release assets. Installation prompts privately for the machine token; generated commands never embed it. Existing tokens remain hashed and cannot be recovered. A deliberate reset returns a new token once and invalidates the old one. Deleting a machine requires confirmation and no attached nodes; it never cascades into node/accounting deletion or remotely uninstalls software. The panel URL must be reachable from the node, not localhost. CF/MTG connector installation remains separately documented.

Approach chosen from the user's correction: prebuilt amd64/arm64 packages instead of compiling on node hosts. Package preparation, exact dependency notices/source inventory, verification and publication run in CI. Docker is only used for the panel deployment.

Tasks:
- [ ] Add API regressions for rotation/revocation, permission/origin isolation, deletion with attached nodes, missing IDs and preservation of node/accounting records; implement endpoints.
- [ ] Add command validation/shell quoting tests; implement machine install/rebind dialog and confirmed delete/reset. New machines open the dialog with their one-time token.
- [ ] Prepare native binary release pipeline with checksums and build-specific licenses/source inventory; extend installer with private interactive token input and explicit rebind path. Test the real installer with isolated command/service fakes, including existing-install preservation.
- [ ] Run real PostgreSQL/full frontend/installer checks; one independent final review, fix regressions, publish changes, follow Linux checks and binary release; provide panel update command.

Review focus: concurrent machine deletion/node association; stale machine credentials; URL/shell injection and loopback origins; repeat installation changing a working service; incomplete architecture/license artifacts advertised as available.
