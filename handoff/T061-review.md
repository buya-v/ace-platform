APPROVED

# Review — T061: Admin UI — CSV/PDF Export Utility

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS
The CSV export correctly implements RFC 4180: `\r\n` line endings, double-quote escaping (`""` for embedded quotes), and quoting of values containing commas, quotes, or newlines. `buildCSVString` is a pure function cleanly separated from the DOM/Blob side-effect logic in `exportToCSV`. The print stylesheet correctly hides navigation chrome and the print button itself via both `data-print-hide` and class-based selectors. The `window.print()` call is the correct browser API for triggering print/PDF. The handoff file diff replaces the previous (unrelated domain config) content with accurate T061 content — no stale data remains.

### Security: PASS
No XSS vectors — CSV is generated from in-memory data and downloaded as a Blob, never injected into the DOM as HTML. No user-supplied input is rendered unsanitized. The `exportToCSV` function uses `URL.createObjectURL` + `URL.revokeObjectURL` correctly, preventing memory leaks. No credentials or secrets are involved.

### Code Quality: PASS
- Clean separation of pure logic (`buildCSVString`) from side effects (`exportToCSV`) follows good functional design.
- `CsvColumn` interface is well-typed and exported for reuse.
- Print CSS uses `[class*="sidebar"]` broad selectors — reasonable approach for CSS Modules' hashed class names.
- TopBar change is minimal and non-invasive (adds button before existing status display).
- Unicode printer icon (`&#128424;`) avoids adding icon dependencies — pragmatic choice consistent with the zero-heavy-dep frontend pattern.

### Test Coverage: PASS
12 tests covering:
- Empty data (header-only output)
- Single and multiple rows
- Comma, double-quote, and newline escaping
- Null, undefined, and missing key handling
- Header escaping
- DOM-based export flow (Blob creation, link click, cleanup, revokeObjectURL)

100% coverage on `export.ts` as claimed. Tests assert meaningful behavior (exact CSV string content, Blob type, cleanup lifecycle), not just "runs without error."

## Required Fixes
None.

## Suggestions (non-blocking)

1. **CSV formula injection** — Values starting with `=`, `+`, `-`, `@`, `\t`, `\r` can trigger formula execution when opened in Excel/Sheets. Consider prefixing such values with a single quote (`'`) or a tab character inside the quoted field in `escapeCSVValue`. Low risk since this is admin-only data exported by authenticated admins, but worth hardening if the export is ever exposed to untrusted data.

2. **BOM for Excel compatibility** — Prepending a UTF-8 BOM (`\uFEFF`) to the CSV string would improve Excel's handling of non-ASCII characters. Minor quality-of-life improvement.

3. **Print stylesheet link path** — `index.html` references `/src/styles/print.css` which works in Vite dev mode (source files served directly) but should be verified in production builds to ensure the CSS is bundled correctly by Vite since it's loaded via a `<link>` tag rather than an ES import.
