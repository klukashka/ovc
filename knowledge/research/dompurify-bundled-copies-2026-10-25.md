# DOMPurify copies bundled inside the diagram renderer

Checked 2026-10-25. The documentation checks render every Mermaid diagram through `@mermaid-js/mermaid-cli` 12.0.0 in `tools/docs`, which loads mermaid's and the ZenUML plugin's self-contained browser bundles. Those bundles carry their own copies of DOMPurify, which OSV-Scanner cannot see in `tools/docs/pnpm-lock.yaml`; the lockfile's own `dompurify` package, now 3.4.17, is not what the renderer runs.

## Bundled versions

Read from the `@license DOMPurify` banner and the `version` constant in each built file of the installed packages:

| Package | Built files | Bundled DOMPurify |
|---|---|---|
| `mermaid` 12.1.0 | `dist/mermaid.js`, `dist/mermaid.min.js` and the ESM chunks | 3.4.12 |
| `@zenuml/core` 3.50.1 | `dist/zenuml.js`, `dist/zenuml.esm.mjs` | 3.3.1 |
| `@mermaid-js/mermaid-zenuml` 1.0.1 | `dist/mermaid-zenuml.js`, `dist/mermaid-zenuml.min.js` and the ESM chunks | 3.3.1 |

## Advisories that reach the bundled copies

Every advisory that OSV returned on 2026-10-25 for `dompurify` 3.4.12 or 3.3.1 (`POST https://api.osv.dev/v1/query` with the package, the npm ecosystem and the version); 22 in all, 5 of them reach the 3.4.12 copy inside mermaid. The fixed version is the first release without the flaw in the range that contains the bundled version; one advisory records only a last affected version.

| Advisory | Severity | Summary | Fixed in | 3.4.12 (mermaid) | 3.3.1 (ZenUML) |
|---|---|---|---|---|---|
| GHSA-cj63-jhhr-wcxv | moderate | USE_PROFILES prototype pollution allows event handlers | 3.3.2 | not affected | affected |
| GHSA-cjmm-f4jc-qw8r | moderate | ADD_ATTR predicate skips URI validation | 3.3.2 | not affected | affected |
| GHSA-h8r8-wccr-v5f2 | moderate | DOMPurify is vulnerable to mutation-XSS via Re-Contextualization | 3.3.2 | not affected | affected |
| GHSA-v2wj-7wpq-c8vv | moderate | DOMPurify contains a Cross-site Scripting vulnerability | 3.3.2 | not affected | affected |
| GHSA-39q2-94rc-95cp | moderate | DOMPurify's ADD_TAGS function form bypasses FORBID_TAGS due to short-circuit evaluation | 3.4.0 | not affected | affected |
| GHSA-crv5-9vww-q3g8 | moderate | DOMPurify has a SAFE_FOR_TEMPLATES bypass in RETURN_DOM mode | 3.4.0 | not affected | affected |
| GHSA-h7mw-gpvr-xq4m | moderate | FORBID_TAGS bypassed by function-based ADD_TAGS predicate (asymmetry with FORBID_ATTR fix) | 3.4.0 | not affected | affected |
| GHSA-v9jr-rg53-9pgp | moderate | Prototype Pollution to XSS Bypass via CUSTOM_ELEMENT_HANDLING Fallback | 3.4.0 | not affected | affected |
| GHSA-hpcv-96wg-7vj8 | moderate | Cross-realm IN_PLACE sanitization leaves executable markup intact via realm-bound `instanceof` checks | 3.4.6 | not affected | affected |
| GHSA-r47g-fvhr-h676 | moderate | IN_PLACE mode preserves attributes of a clobbered root element, allowing XSS via attacker-controlled root DOM | 3.4.6 | not affected | affected |
| GHSA-x4vx-rjvf-j5p4 | low | `IN_PLACE` mode trusts attacker-controlled `nodeName` on live non-form nodes, allowing script retention and XSS via attacker-supplied DOM objects | none recorded; last affected 3.4.6 | not affected | affected |
| GHSA-76mc-f452-cxcm | moderate | Hook mutation of `data.allowedTags` / `data.allowedAttributes` permanently pollutes `DEFAULT_ALLOWED_TAGS` / `DEFAULT_ALLOWED_ATTR` | 3.4.7 | not affected | affected |
| GHSA-rp9w-3fw7-7cwq | moderate | IN_PLACE Sanitization Bypass via Attached Shadow Root Inside <template>.content | 3.4.7 | not affected | affected |
| GHSA-gvmj-g25r-r7wr | low | SAFE_FOR_TEMPLATES bypass - template expressions survive sanitization inside <template> content when using DOM output modes | 3.4.8 | not affected | affected |
| GHSA-vxr8-fq34-vvx9 | low | Trusted Types policy survives `clearConfig()` and can poison later `RETURN_TRUSTED_TYPE` output | 3.4.9 | not affected | affected |
| GHSA-cmwh-pvxp-8882 | moderate | Permanent `ALLOWED_ATTR` pollution via `setConfig()` bypassing the hook clone-guard (incomplete fix of the 3.4.7 hook-pollution patch) | 3.4.11 | not affected | affected |
| GHSA-c2j3-45gr-mqc4 | low | `CUSTOM_ELEMENT_HANDLING` bypasses `afterSanitizeElements` for allowed custom elements. | 3.4.12 | not affected | affected |
| GHSA-55q2-fjhq-7xh7 | moderate | IN_PLACE hook removal leaves a detached subtree executable, causing XSS | 3.4.13 | affected | affected |
| GHSA-6688-9rhm-gjv2 | low | IN_PLACE returns a force-removed rawtext root whose text carries attacker markup — pure HTML reparse executes | 3.4.16 | affected | affected |
| GHSA-c6gr-qvv4-5v3f | low | SMIL attributeName=href guard is skipped when PARSER_MEDIA_TYPE is application/xhtml+xml | 3.4.17 | affected | affected |
| GHSA-cg2g-xh64-v4q7 | critical | SAFE_FOR_TEMPLATES bypass: split template expressions survive in attached shadow root content on DOM output modes | 3.4.17 | affected | not affected |
| GHSA-mrv2-3pqr-j5jr | high | IN_PLACE: hook that removes a parent or sibling leaves the detached subtree's event handlers armed (incomplete fix for GHSA-p98j-92pf-mc4p) | 3.4.17 | affected | affected |

## Exposure

The findings are accepted on exposure, without an OSV-Scanner entry, which could not match a bundled copy anyway:

- **Input:** the renderer reads only the Markdown that `tools/docs/check-docs.sh` lists from this checkout, the tracked files and the untracked files that Git does not ignore, offline; no input from outside the checkout reaches the bundled copies.
- **Configuration:** no diagram or configuration in the repository sets `dompurifyConfig`, `securityLevel` or an `%%{init}%%` directive, and the check passes the renderer only a Puppeteer configuration. mermaid then sanitizes with the default parser media type and string output, forbidding only `style`, and its hooks only set or remove attributes on links. The renderer loads ZenUML through `@mermaid-js/mermaid-zenuml`, whose definition chunk loads only for a diagram that starts with `zenuml`; the repository has none, and in that chunk the bundled 3.3.1 copy is created but never called. None of these calls use `IN_PLACE`, `SAFE_FOR_TEMPLATES`, `RETURN_DOM`, `USE_PROFILES`, `ADD_TAGS`, `ADD_ATTR`, `CUSTOM_ELEMENT_HANDLING`, `setConfig`, Trusted Types or `application/xhtml+xml`, which most of the advisories above require.
- **Output:** the rendered diagrams are discarded once the check has seen every diagram render.
- **Residual:** the advisories that need no special option, matter only for the 3.4.12 copy inside mermaid, since the 3.3.1 copy is never called; there they could matter only for a diagram crafted by someone who can already write to this checkout, and its output runs in a headless browser whose result is discarded. None of the five advisories that reach 3.4.12 is of that kind: each needs `IN_PLACE`, `SAFE_FOR_TEMPLATES` with DOM output or `application/xhtml+xml`.

## Upgrade path

The bundled copies change only with a release of the package that bundles them:

- `mermaid` 12.2.0 (published 2026-10-13) declares `dompurify` `^3.4.12`; which copy its bundle carries was not checked. `@mermaid-js/mermaid-cli` 12.0.0 is still its latest release.
- `@mermaid-js/mermaid-zenuml` 1.0.1, its latest release, depends on `@zenuml/core` `^3.47.9`, so the 4.x line of `@zenuml/core` (latest 4.11.0) is out of reach until the plugin moves.

Recheck the bundled versions whenever the documentation tools are upgraded.
