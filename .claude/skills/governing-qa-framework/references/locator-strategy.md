# Locator strategy & web-first assertions

Tests rot when they depend on brittle selectors and sleeps. The framework should make the robust path
the easy path.

## Locator priority (most → least robust)
1. **Role**: `getByRole('button', { name: 'Send' })` — matches how users and assistive tech perceive the
   UI; survives restyling.
2. **Label/text for content**: `getByLabel('Prompt')`, `getByText(...)` for stable copy.
3. **Test id**: `getByTestId('cost-footer')` — add `data-testid` to elements that lack a stable role/name,
   especially dynamic/streamed regions. Cheap insurance for htmx-swapped fragments.
4. **CSS/XPath**: last resort; brittle, flags a missing role/test-id.

## Web-first assertions (auto-waiting)
Use assertions that retry until the condition holds:
```ts
await expect(page.getByTestId('status')).toHaveText(/thinking/);
await expect(page.getByRole('button', { name: 'stop' })).toBeVisible();
```
These wait for the *actual* state. Never `await page.waitForTimeout(500)` — it's a guess that's slow when
unnecessary and flaky when not enough.

## htmx/SSE specifics for this app
- Streamed/swapped fragments arrive async — assert on the post-swap state with a web-first assertion, don't
  sleep for "the swap to finish".
- Counters accumulate in the shared demo session — assert relatively (`toBeGreaterThan(before)`), per the
  authoring-tests demo-gotcha.
- Give streamed regions (`#cur`, `#status`, `#timeline`, inline forms) stable test ids so locators don't
  depend on htmx-generated structure.

## Enforce it
`framework-audit.sh` greps for `waitForTimeout` and raw CSS-heavy locators. Treat any `waitForTimeout` as a
defect to fix, not a style preference.
