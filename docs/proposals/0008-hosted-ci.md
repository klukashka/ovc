# Proposal 0008: Run the merge gate on hosted machines in Russia

- Status: deferred: accepted in principle by the project owner on 2026-11-27, with GitHub as the preferred direction and no work scheduled (pending owner review)
- Reviewed: pending owner review; written 2026-11-27, when the owner asked for research and a proposal
- Author: coding agent
- Date: 2026-11-27
- Trigger: the owner schedules the work. Until then the gate stays on the laptop under ADR-0018.
- Related: [ADR-0018](../decisions/0018-targeted-checks-and-merge-gate.md), [ADR-0008](../decisions/0008-hosting-and-data-residency.md), [ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md), [ADR-0011](../decisions/0011-tiered-verification.md), [NFR-6.1 and NFR-6.2](../requirements/non-functional.md#nfr-6-maintainability-and-verification), [threat model: T-19, T-38, T-39, T-41](../threat-model.md), [hosted CI research](../../knowledge/research/hosted-ci-2026-11-27.md)

## Motivation

Before every merge, the merge gate ([ADR-0018](../decisions/0018-targeted-checks-and-merge-gate.md)) runs the full local check run on the owner's laptop.

- A full-run gate takes about 600 to 670 s.
- The coordinator runs about 15 to 20 gates a day, so the laptop is busy with gates for about 2.5 to 3.7 hours a day.
- While a gate runs, ADR-0018's idle-machine rule forbids every other check, test, build and measurement on the laptop. Parallel sessions queue for slots, and implementing agents wait to run their change checks.
- Gates now exceed NFR-6.2's 600 s limit, and every passing gate of 600 s or more goes to the owner.

The repository has no remote either: the only copy of `main` is on the laptop. `AGENTS.md` expects a hosted remote and CI one day, but no decision says how.

If nothing is done, the queue grows with the number of sessions, and gates keep running on the hardware that also runs the sessions.

## Proposal

**Recommendation: option A.** Every merge gate runs on a fresh, throwaway virtual machine in a Russian cloud. The VM is built from a pinned image and deleted after the gate. The laptop keeps the gate's locks, its records and the finishing step.

This option has three advantages:

- It needs no forge account and sends nothing abroad.
- A candidate that runs hostile code can affect only its own run, because no machine survives from one gate to the next.
- Parallel gates later just mean more VMs.

### How a gate runs

1. **Laptop side, before the checks.** The coordinator runs `main`'s `tools/merge-gate.sh` in a new remote mode, as today from a scratch folder. That side keeps every step that does not run checks:
   - It takes the merge-gate lock.
   - It refuses an unclean tree or refused settings.
   - It records `main`'s commit and the tested tree. For a squash merge, that tree is still `git write-tree` of the squashed index, staged on `main` as `AGENTS.md` requires.
   - It packs the tested tree with its history into a Git bundle. For a squash, it first makes a commit of that exact tree with `git commit-tree` and points a temporary reference outside `refs/heads/` at it, for example `refs/gate/<id>`, so the bundle can name it. The reference is deleted after the gate; no branch or index changes.
2. **The VM.** A wrapper starts a VM from the pinned image, with a key created for that VM alone. The cloud API pins the VM's SSH host key; it is never trusted on first use.
   - The VM's firewall accepts only SSH and allows outbound connections only to the package, image, browser and vulnerability-database sources the checks need.
   - Docker publishes service ports on loopback only.
3. **Checks on the VM.** The laptop streams `main`'s copy of the gate's check part to the VM and starts it on a checkout of the bundle. The gate itself never comes from the candidate.
   - The VM generates its own test-only settings with random values at boot. It writes them where `tools/dev-env.sh` and `tools/check-dev-env.sh` expect them on the VM, or the wrapper gains a VM mode; the ADR decides which. The settings exist only on that VM. No owner environment file and no agent-written file is copied there.
   - The lanes, the end-to-end run, the clean-tree and hash checks, and the attempt records run as today.
4. **Laptop side, after the checks.** The laptop collects the command logs, the per-check exit records and the commit and tree the VM checked out. It refuses the result if that commit or tree differs from what it sent, then deletes the VM.
   - The gate passes or fails on the laptop. The result and the logs go into the laptop's attempt log, with the VM's identifier and image.
5. **Merge and finish.** On a pass, the coordinator merges as today: a fast-forward to the tested commit, or the squash commit of the staged tree. Then the laptop's finishing step proves that `main^{tree}` equals the tested tree, unchanged.
   - Phase 1 adds no Git push. The candidate travels as a bundle over the per-VM SSH channel.

### Hosted remote

A copy of `main` off the laptop is a separate, smaller step that the owner may take in the same change or later. After each passed gate and its finishing step, the coordinator writes a Git bundle of `main` to a private bucket in Russian object storage. No forge and no push permission is involved, and the owner can restore from any bundle.

A forge remains the right step when a second person joins or pull-request review is wanted, as the last bullet of `AGENTS.md`'s Change workflow section already foresees.

### Measurement and switch-over

1. Five passing gates on the VMs, run one at a time, set the new baseline wall time. That wall time runs from the gate's start line on the VM to its last check, as today. The VM's boot is reported separately.
2. Each of the first five trees is also gated on the laptop, and both outcomes are recorded.
3. A divergence, for example a WebKit failure that only Linux shows, is fixed as a flaky-test bug before the VM gate becomes the only gate.

### Phase 2: parallel gates, only if measured to help

Phase 2 starts only if, after phase 1, changes still wait in the queue longer than a gate lasts. Two candidates are then gated at once on two VMs as a stack:

- the second candidate's tree is built on top of the first;
- both merge in order;
- if the first fails, the second is rebuilt on `main` and gated again.

This works like a small merge queue and needs its own decision.

### Costs

These figures are from the research and are **calculated**.

- A 16 vCPU, 32 GB VM billed per second costs 30.40 ₽ an hour.
- At an assumed 15 minutes per gate including boot, 450 to 600 gates a month come to about 3,400 to 4,600 ₽, plus the image and disk storage.
- The boot and image-pull time is not yet measured.

### What does not change

The following stay as they are:

- the tiers, reviewers and security trigger of [ADR-0011](../decisions/0011-tiered-verification.md);
- the content of every check;
- the rule that the gate is `main`'s copy;
- the rerun and after-fix rules, except that VM and network failures join the named infrastructure errors;
- one gate per merge in phase 1;
- the merge report, which adds the VM's identifier and image;
- the laptop's change checks.

Neither the repository nor a gate run holds data of OVC's users ([research, section 6](../../knowledge/research/hosted-ci-2026-11-27.md#6-data-residency-under-adr-0008)). The commit history holds the owner's own name and email, which stay in Russia under option A.

## Changes to the guarantees, stated openly

- **Forged results on Linux.** ADR-0018 records that on Linux a lane command can reach the gate's result pipe through `/proc` and forge its record. On the macOS laptop the pipe resists this. Two things narrow the gap, though neither fully closes it:
  - The VM runs the checks as a user other than the one that records the results, with `hidepid` and a restricted `ptrace_scope`.
  - The checks run without Docker access wherever the test fixture can use services started beforehand.

  Docker access is root-equivalent, so a check that keeps it could forge its own run. T-41 records this as a residual risk, accepted by the owner when the decision is taken: a candidate could fake its own pass. Two things bound that risk. The VM is thrown away, so the candidate cannot affect any other gate. And the readiness reviews, which see the diff, come before the gate.
- **A remote reports the result.** The result is now produced by a machine that the candidate's code has run on. The laptop checks the reported commit and tree, and the VM never outlives its gate.

## Alternatives

| Option | Monthly cost (research figures) | Fit with ADR-0018 | Main drawbacks |
|---|---|---|---|
| **A. Throwaway VM per gate in a Russian cloud** (recommended) | about 3,400 to 4,600 ₽ (**calculated**) | The laptop keeps locks, records and the finishing step; the gate script gains a remote mode | A cloud API key on the laptop; the weaker Linux result pipe; boot time not yet measured |
| A2. One persistent gate host in Russia | about 15,900 ₽ for a dedicated 10-core server (location and VAT unverified) | Same as A | A candidate with Docker access is root on the host and can tamper with every later gate; rebuilding helps only after someone notices |
| B. GitHub private repository with a self-hosted runner on a Russian host | the host as in A2; GitHub Free costs nothing | Needs a workflow dispatched from `main` that checks `github.ref`, since a push-triggered workflow lets a change edit its own checks; Free cannot enforce required checks on a private repository | Code, the owner's commit identity and runner logs stored abroad (logs go to Microsoft and Amazon storage); sanctions-driven suspension; paying for Pro or Team from Russia is unclear; a persistent runner with Docker access is persistently compromisable; the postponed per-minute charge may return |
| C. GitHub hosted larger runners | about 4,500 to 6,700 minutes on a 16-core runner at $0.042: $189 to $281, plus Team seats | As B; a merge queue that tests the merged tree needs Enterprise Cloud | Payment from Russia, cost, and the same exposure abroad |
| D. GitVerse with a self-hosted runner | the host as in A2; GitVerse is free | As B, if its trigger can be pinned to `main` | Young service; cloud runners stop at 30 minutes and have no Docker; where it stores data is not stated; the runner risks of B |
| E. Self-hosted GitLab CE or GitFlic Free on a Russian host | a host larger than A2's, or about 6,400 to 6,700 ₽ for Yandex Managed GitLab, plus runners | As B; merge trains and merged results pipelines need paid tiers | A second internet-facing service to patch and secure, for features OVC does not use yet |
| F. Keep the laptop gate | none | Today's rules | The queue and the 600 s question remain |

## Plan

Every step below meets the security trigger, because each changes verification policy, agent configuration or a trust boundary. Each task plan therefore gets a standard reviewer and a security-profile reviewer.

1. **Decision record.** A new ADR records the remote gate and amends ADR-0018. It covers:
   - the VM's residency, image and lifetime;
   - the remote mode of `tools/merge-gate.sh`: what stays on the laptop, the bundle, how the gate's own copy is streamed, and how the reported commit and tree are checked;
   - the idle-machine rule restated for the VM;
   - the 600 s budget measured on the VM;
   - the extra named infrastructure errors;
   - the Linux result-pipe residual risk;
   - the merge report's new fields.
2. **Secrets.** The cloud API key is a service-account key limited to one folder, with compute rights and a quota cap. It lives in the owner's home directory and is used only through a wrapper like `tools/dev-env.sh`; agents never read it. The amendments:
   - ADR-0009 item 7 is extended to cover that key and the per-VM keys;
   - the Secrets bullet of `AGENTS.md` names the wrapper;
   - the VM's generated settings are recorded as never leaving the VM.
3. **Agent configuration.** In the same change as the ADR:
   - `.claude/settings.json` gains ask or deny entries for direct use of the cloud's command-line tool and for the key's path, so agents reach the cloud only through the remote-gate wrapper. Every `git push` keeps its confirmation, since phase 1 pushes nothing.
   - The secret guard hook covers the key's location.
   - The `AGENTS.md` Change workflow and Development verification sections drop "local only" and point the idle-machine rule and the check budget at the VM. The rule that every after-fix evidence run and measurement run goes through the main checkout's `tools/dev-env.sh` names the remote-gate wrapper as the other allowed route for runs on the VM.
   - The AI development inventory in `knowledge/reviews/ai-development-setup.md` lists the new wrapper and permission.
4. **Requirements and architecture.**
   - NFR-6.1 and NFR-6.2 name the gate's reference machine.
   - ADR-0008 records the development-infrastructure host and that it holds no user data.
   - The stack table in `knowledge/architecture/detailed.md` adds the provider and image.
   - The development guide and the ovc-verify skill describe the remote gate.
5. **Threat model.**
   - **New threat:** a compromised VM image, cloud key or per-VM key that forges a pass or reads the repository. Mitigations: a pinned image built from `main`'s copy of the provisioning script, so a candidate cannot change the image it is gated on; per-VM keys; host keys pinned through the API; a quota cap; rotation and revocation of the cloud key.
   - **Exfiltration by candidate code:** a candidate could upload the repository through an allowlisted host such as a package registry, using an account of its own. This is a residual risk: the repository holds no user data or secrets, and the readiness reviews read the diff before the gate.
   - **T-41:** the residual risk above, and the result that now arrives over SSH.
   - **T-38:** the gate's development services and test-control routes on a VM with a public address, behind the firewall above.
   - **T-39:** the image's provisioning. No `curl | sh`, a pinned base image, Docker from its signed repository with the key fingerprint recorded, checksums for the Go and Node archives, and the recorded gap that Playwright's browser downloads come without checksums.
   - **T-19:** the cloud key and the per-VM keys.
6. **Measurement and switch-over** as above.
7. **Phase 2**, only if its trigger is met, as its own proposal or ADR.

## Risks and open questions

- **Payment and provider.** The owner chooses the provider, creates the cloud account and pays. No agent creates an account or enters payment details. Hardware prices in Russia are rising, so the research figures may already be stale.
- **Noisy neighbours.** Whether a full-share vCPU on the chosen cloud is free of other tenants' load is unverified. A dedicated-host option, or A2 on bare metal, is the fallback if measurements stay noisy.
- **Downloads from Russia.** Whether Docker Hub images, Playwright's browsers, Go and npm modules and the vulnerability databases download reliably on a Russian VM is unverified. Building them into the image reduces the per-gate risk. A registry mirror would itself become a T-39 dependency.
- **Docker access in the checks.** Whether the integration-test fixture can run without Docker access, and whether its host check still passes on the VM (T-36), is unknown until the ADR's task plan checks it.
- **Linux differences.** Linux WebKit and fonts may fail tests that pass on macOS, and the reverse. The first gates run on both machines.
- **Budget.** A VM slower than the laptop could push the gate above 600 s. The measurement step decides this before the switch.
- **A forge now.** If the owner prefers GitHub now, option B with a workflow dispatched from `main` that checks its ref is the closest fit. The owner would first have to accept the payment, sanctions and storage-abroad risks.

## Owner's response of 2026-11-27

The project owner answered the questions this proposal raised on 2026-11-27 (pending owner review):

- **Direction.** GitHub is acceptable, and is the owner's preferred direction: option B, a private repository on GitHub. Running the gate on a self-hosted runner on a Russian virtual server is the variant that avoids paying for GitHub's hosted minutes. The risks of option B recorded above stay open, and the follow-up decision record has to address them:
  - code, the owner's commit identity and runner logs are stored abroad;
  - paying for a plan or minutes from Russia is unclear, and GitHub Free cannot enforce required checks on a private repository;
  - an account can be suspended under sanctions;
  - a self-hosted runner with Docker access can be persistently compromised, and the workflow must be dispatched from `main` and check its ref, so that a change cannot edit its own gate.
- **The Linux result-pipe residual risk.** No preference; the follow-up decision decides it.
- **A copy of `main` off the laptop.** Later.
- **Next step.** The proposal is accepted in principle only. No decision record, task plan or setup work is scheduled now. This file keeps option A as the agent's analysis next to the owner's preferred direction.

## Decision

Not decided.
